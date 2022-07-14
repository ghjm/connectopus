Connectopus began as a personal project for exploring how different design decisions might have played out as compared to
[Receptor](https://github.com/ansible/receptor), which I wrote while working at Red Hat.  The major differences are:

* Both Connectopus and Receptor have nodes with names, and want a convenient way of addressing them.  Receptor accomplishes this by having the node name (or, technically, a [HighwayHash](https://github.com/google/highwayhash) of the node name) just literally be the address.  Connectopus, instead, assigns each node an IPv6 address, and uses a built-in DNS server to provide node addressing by name.
* Receptor doesn't have a good way of allowing out-of-process access to the mesh network.  As a result, applications that run on top of Receptor (notably Workceptor) have to be compiled in to Receptor, meaning their code has to live in the Receptor repo, they have to run inside the single Receptor process, and so on.  Connectopus instead uses IPv6 networking, so applications can run in a separate process and listen on a port, the way network applications normally do.
* Because of the above, Receptor has to provide a centralized scheme for backend communication with applications - the controlsvc.  Connectopus has no need of this.
* Receptor is configured using a YAML file per node.  If you want to reconfigure a Receptor mesh, you have to go to each node, change its config file, and restart - Receptor itself gives you no help with this.  Connectopus uses a single YAML file, automatically distributed to all nodes, that configures the whole mesh.
* Receptor mesh sizes are inherently limited by the fact that routing updates are sent over a backend connection with a limited packet size.  Even with fragmentation, Receptor routing updates cannot be larger than 64k.  Connectopus uses an out-of-band stream protocol to allow arbitrarily large routing updates, which matters because Connectopus, unlike Receptor, sometimes includes its configuration YAML in a routing update.
* Because Receptor expects to find its config files on the local filesystem, it can assume they are secure and untampered-with.  Connectopus can get configuration over the network, so it needs a way to ensure authenticity.  This is accomplished using SSH signatures.
* To use TLS above the mesh in Receptor, you need a special type of certificate using a custom OID in the X.509 otherName field.  Connectopus doesn't require this.
* If you want to transport a non-in-process service over Receptor, you need (at least) three TCP proxies: client-nodeA, nodeA-nodeB and nodeB-server.  Each of these requires TLS termination, one of which has to use the special otherName certificates.  Connectopus can route layer 3 traffic, so with Connectopus you can have a TLS connection where the endpoints are the ultimate client and server.

The major open problems I'm still interested in working on are:

* Performance.  On my machine, Connectopus can manage about 300Mbps over DTLS and 900Mbps over TCP (slightly beter than Receptor).  I think this can be drastically improved - packet processing is currently single threaded, and there are currently a lot of memory allocations and maybe lock contention on the hot path.
* Dynamic node addressing.  Currently, Connectopus nodes have to be given a static address.  This is a problem if you want to run Connectopus inside Kubernetes containers where they will be created and destroyed.  It should be possible for one Connectopus node to issue addresses to another, possibly using DHCPv6.
* Dynamic subnet addressing.  Currently, you have to configure a non-routable static IPv6 subnet for the whole Connectopus mesh.  Ideally, Connectopus should be able to request an IPv6 subnet using DHCPv6 or SLAAC.  This has some design implications, like deciding which node is the master node that ought to be requesting addresses for the whole mesh.
* Writing a CNI network plugin to allow Docker, Podman and Kubernetes to launch containers where the container's native network _just is_ the Connectopus mesh.