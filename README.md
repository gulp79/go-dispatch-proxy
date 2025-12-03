# Go dispatch proxy
A fork of https://github.com/extremecoders-re/go-dispatch-proxy

🚀 Major Code Refactoring and Robustness Improvements
This fork introduces a significant refactoring of the codebase, transitioning from a C-style Go implementation to a more Idiomatic and Robust Go structure. The primary goals were to enhance thread safety, network reliability, testability, and adherence to modern Go conventions.

🌟 Key Changes and Improvements
1. Architectural and State Management (Thread Safety)
The original code relied heavily on global state, making concurrent access brittle and difficult to test.

Elimination of Global State: Removed global variables (lb_list, lb_index, mutex) from main.go.

Encapsulated Dispatcher: Introduced the Dispatcher struct (balancer.go). This structure encapsulates the load balancer list and the selection logic, protecting access via an internal sync.Mutex. This ensures thread-safe load balancing in a high-concurrency environment.

New File: Added balancer.go to handle all load balancing logic, cleanly separating it from the main server setup.

2. SOCKS5 Protocol Handling (Robustness)
Network applications must be resilient to partial reads and unexpected client behavior.

Guaranteed Reads (io.ReadFull): The logic in socks.go was vulnerable to partial TCP reads (conn.Read). This has been replaced with io.ReadFull in all critical sections (handshake, request parsing) to guarantee that the expected number of bytes are read, significantly improving protocol compliance and preventing subtle network bugs.

Improved Error Handling: Added robust error responses to the client (using ReplyError) upon request failure.

IPv6 Support: Added basic parsing logic for SOCKS5 IPv6 address types (AddrTypeIPv6) in socks.go.

3. Concurrency and Resource Management
Connection Deadlines (Timeouts): Introduced explicit 10-second Dial Timeout in DialBackend (dialer_*.go). This prevents goroutine blocking and resource exhaustion if a backend server is slow or unreachable.

Clean Pipe Management: The PipeConnections function in main.go was updated to use a clean synchronization channel (done) and CloseWrite where possible, ensuring that both connection sides are properly signaled before final closure, minimizing connection leaks.

4. Code Style and Conventions
The code now adheres to standard Go idioms:

Naming Conventions: All identifiers have been converted from snake_case (e.g., server_response, load_balancer) to CamelCase (e.g., ServerResponse, LoadBalancer).

Modern Build Tags: Updated build tags in the dialer files from the deprecated // +build format to the modern //go:build format.

5. File Renaming for Clarity




A SOCKS5 load balancing proxy to combine multiple internet connections into one. Works on Windows and Linux. [Reported to work on macOS](https://github.com/extremecoders-re/go-dispatch-proxy/issues/1). Written in pure Go with no additional dependencies.

It can also be used as a transparent proxy to load balance multiple SSH tunnels.

## Rationale

The idea for this project came from [dispatch-proxy](https://github.com/Morhaus/dispatch-proxy) which is written in NodeJS.
[NodeJS is not entirely disk friendly considering the multitude of files it creates even for very simple programs](https://medium.com/@jdan/i-peeked-into-my-node-modules-directory-and-you-wont-believe-what-happened-next-b89f63d21558). I needed something light & portable, preferably a single binary without polluting the entire drive.

## Installation

No installation required. Grab the latest binary for your platform from the [CI server](https://ci.appveyor.com/project/extremecoders-re/go-dispatch-proxy/build/artifacts) or from [releases](https://github.com/extremecoders-re/go-dispatch-proxy/releases) and start speeding up your internet connection!

[![Build status](https://ci.appveyor.com/api/projects/status/nll4hvpdjlfsp7mu?svg=true)](https://ci.appveyor.com/project/extremecoders-re/go-dispatch-proxy/build/artifacts)

## Usage

The example below are shown on Windows. The steps are similar for other platforms.

### 1 - Load balance connections

The primary purpose of the tool is to combine multiple internet connections into one. For this we need to know the IP addresses of the interface we wish to combine. You can obtain the IP addresses using the `ipconfig` (`ip a` on linux) command. Alternatively run `go-dispatch-proxy -list`.

```
D:\>go-dispatch-proxy.exe -list
--- Listing the available adresses for dispatching
[+] Mobile Broadband Connection , IPv4:10.81.201.18
[+] Local Area Connection, IPv4:192.168.1.2
```

Start `go-dispatch-proxy` specifying the IP addresses of the load balancers obtained in the previous step. Optionally, along with the IP address you may also provide the contention ratio(after the @ symbol). If no contention ratio is specified, it's assumed as 1.

### 2 - Load balance SSH tunnels

The tool can load balance multiple SSH tunnels. See Example 3 for usage.

### Example 1

SOCKS proxy running on localhost at default port. Contention ratio is specified.
```
D:\>go-dispatch-proxy.exe 10.81.201.18@3 192.168.1.2@2
[INFO] Load balancer 1: 10.81.201.18, contention ratio: 3
[INFO] Load balancer 2: 192.168.1.2, contention ratio: 2
[INFO] SOCKS server started at 127.0.0.1:8080
```

### Example 2

SOCKS proxy running on a different interface at a custom port. Contention ratio is not specified.

```
D:\>go-dispatch-proxy.exe -lhost 192.168.1.2 -lport 5566 10.81.177.215 192.168.1.100
[INFO] Load balancer 1: 10.81.177.215, contention ratio: 1
[INFO] Load balancer 2: 192.168.1.100, contention ratio: 1
[INFO] SOCKS server started at 192.168.1.2:5566
```

Out of 5 consecutive connections, the first 3 are routed to `10.81.201.18` and the remaining 2 to `192.168.1.2`. The SOCKS server is started by default on `127.0.0.1:8080`. It can be changed using the `-lhost` and `-lport` directive.

Now change the proxy settings of your browser, download manager etc to point to the above address (eg `127.0.0.1:8080`). Be sure to add this as a SOCKS v5 proxy and NOT as a HTTP/S proxy.

### Example 3

The tool can be used to load balance multiple SSH tunnels. In this mode, go-dispatch-proxy acts as a transparent load balancing proxy. 

First, setup the tunnels. 

```
D:\> ssh -D 127.0.0.1:7777 user@192.168.1.100
D:\> ssh -D 127.0.0.1:7778 user@192.168.1.101
```

Here we are setting up two SSH tunnels to remote hosts `192.168.1.100`, and `192.168.1.101` on local ports `7777` and `7778` respectively. The IP address (`127.0.0.1`) if omitted defaults to localhost. The `-D` option stands for dynamic port forwarding.

Next, launch go-dispatch-proxy using the `-tunnel` argument. 

```
D:\> go-dispatch-proxy.exe -tunnel 127.0.0.1:7777 127.0.0.1:7778
```

Both the IP and port must be mentioned while specifying the load balancer addresses. Also instead of specifying the IP address a domain can be specified, hence the following also works.

```
D:\> go-dispatch-proxy.exe -tunnel proxy1.com:7777 proxy2.com:7778
``` 

Optionally, the listening host, port and contention ratio can also be specified like in example 2.

```
D:\> go-dispatch-proxy.exe -lport 5555 -tunnel 127.0.0.1:7777@1 127.0.0.1:7778@3
```

The `lport` if not specified defaults to 8080. This is the port where you need to point your web browser, download manager etc. Be sure to add this as a SOCKS v5 proxy.

## Full Linux Support [NEW]

Go-dispatch-proxy now supports Linux in both normal mode and tunnel mode. On Linux normal mode, Go-dispatch-proxy uses the `SO_BINDTODEVICE` syscall to bind to the interface corresponding to the load balancer IPs. As a result, the binary must be run with `root` privilege or by giving it the necessary capabilities as shown below.

```
$ sudo ./go-dispatch-proxy
```

OR (Recommended)

```
$ sudo setcap cap_net_raw=eip ./go-dispatch-proxy
$ ./go-dispatch-proxy
```

Tunnel mode doesn't require root privilege.

## Compiling (For Development)

Ensure that Go is installed and available on the system path.

```sh
$ git clone https://github.com/extremecoders-re/go-dispatch-proxy.git
$ cd go-dispatch-proxy

# Compile for Windows x86
$ GOOS=windows GOARCH=386 go build

# Compile for Windows x64
$ GOOS=windows GOARCH=amd64 go build

# Compile for Linux x86
$ GOOS=linux GOARCH=386 go build

# Compile for Linux x64
$ GOOS=linux GOARCH=amd64 go build

# Compile for Macos x64
$ GOOS=darwin GOARCH=amd64 go build
```

## Credits

- [dispatch-proxy](https://github.com/Morhaus/dispatch-proxy): A SOCKS5/HTTP load balancing proxy written in NodeJS.

## License
