//go:build cgo && !netgo && linux

package main

/*
#include <sys/types.h>
#include <sys/socket.h>
#include <netdb.h>

typedef const char *restrict c_char_r_ptr_t;
typedef const struct addrinfo *restrict c_addrinfo_r_ptr_t;
typedef struct addrinfo **c_addrinfo_ptr_ptr_t;
typedef struct addrinfo *c_addrinfo_ptr_t;
typedef struct sockaddr_in *c_sockaddr_in_ptr_t;
*/
import "C"

import (
	"encoding/binary"
	"github.com/rainycape/dl"
	"log"
	"net"
	"strings"
	"unsafe"
)

func main() {}

//export getaddrinfo
func getaddrinfo(node C.c_char_r_ptr_t, service C.c_char_r_ptr_t, hints C.c_addrinfo_r_ptr_t, res C.c_addrinfo_ptr_ptr_t) C.int {
	nodeStr := C.GoString(node)
	var ret int
	if strings.EqualFold(nodeStr, "example.com") {
		m := C.malloc(C.sizeof_struct_addrinfo + C.sizeof_struct_sockaddr_in)
		ai := C.c_addrinfo_ptr_t(m)
		ai.ai_family = C.AF_INET
		ai.ai_socktype = C.SOCK_STREAM
		saiU := unsafe.Add(m, int(C.sizeof_struct_addrinfo))
		sai := C.c_sockaddr_in_ptr_t(saiU)
		sai.sin_family = C.AF_INET
		addr := net.ParseIP("10.1.2.3")
		if len(addr) == 16 {
			sai.sin_addr.s_addr = C.uint(binary.LittleEndian.Uint32(addr[12:16]))
		} else {
			sai.sin_addr.s_addr = C.uint(binary.LittleEndian.Uint32(addr))
		}
		ai.ai_addrlen = C.sizeof_struct_sockaddr_in
		ai.ai_addr = (*C.struct_sockaddr)(saiU)
		*res = ai
		ret = 0
	} else {
		ret = oldGetaddrinfo(node, service, hints, res)
	}
	return C.int(ret)
}

var oldGetaddrinfo func(node *C.char, service *C.char, hints *C.struct_addrinfo, res **C.struct_addrinfo) int

func init() {
	lib, err := dl.Open("libc", 0)
	if err != nil {
		log.Fatalln(err)
	}
	defer func() {
		_ = lib.Close()
	}()
	err = lib.Sym("getaddrinfo", &oldGetaddrinfo)
	if err != nil {
		log.Fatalln(err)
	}
}
