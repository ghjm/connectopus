![Connectopus](./docs/logo.png)

foo

#### What is this?

Connectopus is a general-purpose IPv6 overlay network.  The idea is that you run Connectopus nodes, which talk to each other over TCP or UDP connections (including IPv4).  The nodes then form an IPv6 overlay network.  The "above the mesh" network does not necessarily have any outside connectivity.

Key Features:

* Uses SSH keys to authorize configuration updates and UI access
* DTLS and TCP (including TLS) backends
* CLI, GraphQL API and Patternfly UI
* Distributed configuration
* Dynamic routing based on link state and Dijkstra's algorithm
* TUN/TAP interface so you can talk to the Connectopus network from outside (requires root)
* Linux network namespace ability so you can enter the Connectopus network and run processes (does not require root)
* Built-in DNS server so you can refer to nodes by name
* Service manager that provides inetd-like launching of service processes

For more details on the design ideas behind Connectopus, see [DESIGN.md](./docs/DESIGN.md).

#### License

Copyright © 2022 by Graham Mainwaring.  All rights reserved.

This program is free software: you can redistribute it and/or modify it under the terms of the GNU Affero General Public License, version 3, as published by the Free Software Foundation.

This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License along with this program. If not, see <https://www.gnu.org/licenses/>.

#### How do I get it?

Download the [latest build](https://github.com/ghjm/connectopus/releases/tag/latest) for your platform.

#### How can I contribute to it?

See [CONTRIBUTING.md](./CONTRIBUTING.md)

#### How do I run it?

(Note: these instructions assume you are on Linux.  Most, but not all, of this will also work on Windows and Mac.)

* Have an SSH key loaded into an agent (OpenSSH or compatible on Linux/Mac, or Pageant on Windows).  Your SSH key is used to sign configuration bundles to prove their authenticity.

* Review the included [`test.yml`](./test.yml) configuration file.  This configures a three-node network that demonstrates some of the features of Connectopus.

* Launch the first node, `bar`, which is the central listener:
  ```
  ./connectopus init --id bar --config test.yml --run
  ```
  This reads test.yml, signs it, and launches a new node named bar with the given configuration.  If you shut down the node, you can restart it without re-initializing using:
  ```
  ./connectopus node --id bar
  ```

* Launch the other two nodes:
  ```
  ./connectopus init --id foo --subnet fd00::/8 --ip fd00::1:1 --backend 'type=dtls-dialer,peer=localhost:4444,psk=test-psk' --run
  ./connectopus init --id baz --subnet fd00::/8 --ip fd00::3:1 --backend 'type=dtls-dialer,peer=localhost:4444,psk=test-psk' --run
  ```
  Note that we are giving these nodes just enough configuration to reach `bar`, which they will then pull their configuration from.  (We could have just done `--config test.yml` for all three nodes and it would have worked, but this is more fun.)

* At this point, you are probably seeing error messages on `foo` about being unable to configure the ctun tunnel interface.  To fix this, do:
  ```
  sudo ./connectopus setup-tunnel --config test.yml --id foo
  ```
  This will create a tun/tap interface named ctun and configure it as needed.  Once this is done, within a few seconds the `foo` node should launch the tunnel successfully.

* Check out the UI
  ```
  ./connectopus ui --node foo
  ```

* From the shell, try using the tunnel interface to talk to nodes inside the Connectopus mesh:
  ```
  ping fd00::1:1
  traceroute fd00::3:1
  nc fd00::3:1 7
  ```

* Enter a Connectopus network namespace (Linux only):
  ```
  ./connectopus nsenter --node foo
  ```
  then, from inside the subshell, just use normal Linux commands:
  ```
  ifconfig
  traceroute fd00::3:1
  ```
  Note that you can refer to nodes by name using the built-in DNS server:
  ```
  ping bar
  traceroute baz
  ```

* Edit the running configuration:
  ```
  ./connectopus config edit --node foo
  ```
  This downloads the running configuration from foo and opens it.  After you make changes and save, the new configuration is signed and uploaded back to `foo`, and then shared with the rest of the network.  You can add and remove services, namespaces, etc, and they will be restarted and reconfigured on the relevant nodes.  For example, you can copy the service configuration from `baz` to `bar`, after which you can `nc fd00::2:1 7`.  Even though you're talking to `foo`, `bar`'s configuration will be updated.

