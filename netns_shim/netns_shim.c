#define _GNU_SOURCE
#include <sched.h>
#include <stdio.h>
#include <string.h>
#include <errno.h>
#include <stdlib.h>
#include <fcntl.h>
#include <linux/if.h>
#include <linux/if_tun.h>
#include <sys/ioctl.h>
#include <unistd.h>
#include <wait.h>
#include <sys/mman.h>
#include <stdbool.h>
#include <arpa/inet.h>
#include <ev.h>
#include <net/route.h>

void errExit(char *msg, _Bool syserr) {
    char *sysmsg = "";
    if (syserr) {
        asprintf(&sysmsg, ": %s", strerror(errno));
    }
    fprintf(stderr, "Error: %s%s\n", msg, sysmsg);
    exit(1);
}

static int *global_state;

struct in6_ifreq {
    struct in6_addr ifr6_addr;
    u_int32_t ifr6_prefixlen;
    unsigned int ifr6_ifindex;
};

struct ev_io_with_otherfd {
    ev_io io;
    int otherfd;
};

static void watcher_cb(EV_P_ ev_io *w_, int revents) {
    struct ev_io_with_otherfd *w = (struct ev_io_with_otherfd *)w_;
    char buf[1500];
    ssize_t nread = read(w->io.fd, &buf, sizeof(buf));
    if ((nread < 0 && errno == EAGAIN) || (nread == 0)) {
        return;
    } else if (nread < 0) {
        errExit("read error", true);
    }
    ssize_t nwritten = write(w->otherfd, &buf, nread);
    if (nwritten < 0 && errno == EAGAIN) {
        return;
    } else if (nwritten != nread) {
        errExit("write error", true);
    }
}

int setup_networking(char *tun_name, struct sockaddr_in6 ipaddr) {

    // Create network namespace in new rootless user namespace
    int err = unshare(CLONE_NEWNS|CLONE_NEWUSER|CLONE_NEWNET|CLONE_NEWUTS);
    if (err < 0) {
        errExit("unshare", true);
    }

    // Wait for parent process to perform UID mapping
    *global_state = 2;
    while (*global_state < 3) {
        usleep(100);
    }

    // Create socket for use with ioctl
    int sockfd = socket(AF_INET6, SOCK_DGRAM, 0);
    if (sockfd < 0) {
        errExit("allocating socket", true);
    }

    // Bring up the lo interface
    struct ifreq ifr;
    memset(&ifr, 0, sizeof(ifr));
    ifr.ifr_flags = IFF_UP;
    strncpy(ifr.ifr_name, "lo", IFNAMSIZ);
    err = ioctl(sockfd, SIOCSIFFLAGS, &ifr);
    if (err < 0) {
        errExit("configuring lo interface", true);
    }

    // Create /dev/net/tun tunfd for ioctl
    int tunfd = open("/dev/net/tun", O_RDWR);
    if (tunfd < 0) {
        errExit("opening /dev/net/tun", true);
    }

    // Create tun interface
    memset(&ifr, 0, sizeof(ifr));
    ifr.ifr_flags = IFF_TUN|IFF_NO_PI|IFF_UP;
    strncpy(ifr.ifr_name, tun_name, IFNAMSIZ);
    err = ioctl(tunfd, TUNSETIFF, &ifr);
    if (err < 0) {
        errExit("creating tunnel interface", true);
    }

    // Get tun interface index
    memset(&ifr, 0, sizeof(ifr));
    ifr.ifr_flags = IFF_TUN|IFF_NO_PI|IFF_UP;
    strncpy(ifr.ifr_name, tun_name, IFNAMSIZ);
    err = ioctl(sockfd, SIOGIFINDEX, &ifr);
    if (err < 0) {
        errExit("getting tunnel interface index", true);
    }
    int tun_index = ifr.ifr_ifindex;

    // Set tun interface IP address
    struct in6_ifreq ifr6;
    memset(&ifr6, 0, sizeof(ifr6));
    memcpy(&ifr6.ifr6_addr, &ipaddr.sin6_addr, sizeof(struct in6_addr));
    ifr6.ifr6_ifindex = tun_index;
    ifr6.ifr6_prefixlen = 128;
    err = ioctl(sockfd, SIOCSIFADDR, &ifr6);
    if (err < 0) {
        errExit("setting tunnel IP address", true);
    }

    // Bring up tun interface
    memset(&ifr, 0, sizeof(ifr));
    ifr.ifr_flags = IFF_UP;
    strncpy(ifr.ifr_name, tun_name, IFNAMSIZ);
    err = ioctl(sockfd, SIOCSIFFLAGS, &ifr);
    if (err < 0) {
        errExit("configuring tunnel interface", true);
    }

    // Create IPv4 socket for use with ioctl
    int sock4fd = socket(AF_INET, SOCK_DGRAM, 0);
    if (sock4fd < 0) {
        errExit("allocating socket", true);
    }

    // Add IPv4 default gateway
    struct rtentry rt4;
    memset(&rt4, 0, sizeof(rt4));
    rt4.rt_dst.sa_family = AF_INET;
    rt4.rt_flags = RTF_UP;
    rt4.rt_dev = tun_name;
    rt4.rt_metric = 1;
    err = ioctl(sock4fd, SIOCADDRT, &rt4);
    if (err < 0) {
        errExit("adding IPv4 default gateway", true);
    }

    // Add IPv6 default gateway
    struct in6_rtmsg rt6;
    memset(&rt6, 0, sizeof(rt6));
    rt6.rtmsg_flags = RTF_UP;
    rt6.rtmsg_metric = 1;
    rt6.rtmsg_ifindex = tun_index;
    err = ioctl(sockfd, SIOCADDRT, &rt6);
    if (err < 0) {
        errExit("adding IPv6 default gateway", true);
    }

//    struct sockaddr_in *addr = (struct sockaddr_in *)&rt4.rt_dst;
//    addr->sin_family=AF_INET;
//    addr->sin_addr.s_addr=INADDR_ANY;
//    rt4.rt_dev = "tun9";
//    rt4.rt_flags = RTF_UP | RTF_GATEWAY;
//    rt4.rt_metric = 1;

    close(sock4fd);
    close(sockfd);

    return tunfd;
}

void run_eventloop(int tunfd, int netfd) {

    // Set up the event loop
    struct ev_loop *loop = EV_DEFAULT;

    // Set the fds to non-blocking
    int flags;
    flags = fcntl(tunfd, F_GETFL, 0); fcntl(tunfd, F_SETFL, flags | O_NONBLOCK);
    flags = fcntl(netfd, F_GETFL, 0); fcntl(netfd, F_SETFL, flags | O_NONBLOCK);

    // Set up the tunfd watcher
    struct ev_io_with_otherfd tunfd_watcher;
    tunfd_watcher.otherfd = netfd;
    ev_io_init(&tunfd_watcher.io, watcher_cb, tunfd, EV_READ);
    ev_io_start(loop, &tunfd_watcher.io);

    // Set up the netfd watcher
    struct ev_io_with_otherfd netfd_watcher;
    netfd_watcher.otherfd = tunfd;
    ev_io_init(&netfd_watcher.io, watcher_cb, netfd, EV_READ);
    ev_io_start(loop, &netfd_watcher.io);

    // Run the loop
    ev_run(loop, 0);
    exit(0);

}

void run_parent(int pid) {
    int status;

    // Wait for child to unshare
    while (*global_state < 2) {
        usleep(100);
        int wpid = waitpid(pid, &status, WNOHANG);
        if (wpid != 0) {
            exit(status);
        }
    }

    // Create mapping string
    char *mapping;
    int err = asprintf(&mapping,"0 %d 1", getuid());
    if (err < 0) {
        errExit("mapping", true);
    }

    // Create filename string
    char *filename;
    err = asprintf(&filename,"/proc/%d/uid_map", pid);
    if (err < 0) {
        errExit("filename", true);
    }

    // Open uid_map file
    int fd = open(filename, O_WRONLY);
    if (fd < 0) {
        errExit("opening uid_map file", true);
    }

    // Write mapping to uid_map file
    err = dprintf(fd, "%s\n", mapping);
    if (err < 0) {
        errExit("writing to uid_map file", true);
    }

    // Release the child to set up the network
    *global_state = 3;

    // Notify the caller of the child PID
    printf("Unshared PID: %d\n", pid);

    // Wait for the child to exit
    waitpid(pid, &status, 0);
    exit(status);
}

int main(int argc, char **argv) {

    // command line options
    char *netfd_str = NULL;
    char *tun_name = NULL;
    char *ipaddr_str = NULL;

    int c;
    while ((c = getopt (argc, argv, "f:t:a:")) != -1) {
        switch (c) {
            case 'f':
                netfd_str = optarg;
                break;
            case 't':
                tun_name = optarg;
                break;
            case 'a':
                ipaddr_str = optarg;
                break;
            case '?':
                fprintf(stderr, "Unknown option character `\\x%x'\n", optopt);
                return 1;
            default:
                fprintf(stderr, "Error processing command line parameters\n");
                return 1;
        }
    }

    char *endptr = NULL;
    int netfd;
    if (netfd_str != NULL) {
        netfd = (int) strtoul(netfd_str, &endptr, 10);
    }

    struct sockaddr_in6 ipaddr;
    bool ip_ok = false;
    if (ipaddr_str != NULL) {
        memset(&ipaddr, 0, sizeof(ipaddr));
        ipaddr.sin6_family = AF_INET6;
        ipaddr.sin6_port = 0;
        if (inet_pton(AF_INET6, ipaddr_str, (void *) &ipaddr.sin6_addr) == 1) {
            ip_ok = true;
        }
    }

    if (netfd_str == NULL || tun_name == NULL || !ip_ok || *endptr != 0) {
        fprintf(stderr, "Usage: netns_shim -f <fd> -t <tunnel-name> -a <ip-address>\n");
        return 1;
    }

    global_state = mmap(NULL, sizeof *global_state, PROT_READ|PROT_WRITE, MAP_SHARED|MAP_ANONYMOUS, -1, 0);
    *global_state = 1;

    int pid = fork();
    if (pid < 0) {
        errExit("forking", true);
    }

    if (pid == 0) {
        int tunfd = setup_networking(tun_name, ipaddr);
        run_eventloop(tunfd, netfd);
    } else {
        run_parent(pid);
    }
}
