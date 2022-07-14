## Contributing to Connectopus

#### Licensing

Connectopus is currently licensed under the Affero GPL v3.  If you are interested in contributing, I am asking you to agree to a [Contributor License Agreement](https://gist.github.com/ghjm/359bf26d060e5fd6845ec09f224c9231).  This accomplishes two things for me:

* Perhaps in the future I might want to change to a less restrictive open source license.
* If someone is interested in making commercial use of Connectopus in a way the AGPL doesn't permit, I want to maintain the ability to issue a license exception.

The first time you open a pull request, the CLA Assistant at https://cla-assistant.io will ask you to electronically consent to the agreement.  (Thank you to SAP for providing the community with this useful tool.)

#### Development Environment

* My development environment is Fedora (36 as of this writing).  Ubuntu should also be fine.  If you want to develop on Windows or MacOS you'll be breaking new ground.
* I use the JetBrains GoLand IDE, but nothing in the project should be tied to it.
* Go version 1.18 or better and Node v16 or better are required.  The project makes considerable use of Go generics, so versions before 1.18 definitely won't work.
* The project is set up with pre-commit hooks to run local checks when you create a commit.  To use this, install [pre-commit](https://pre-commit.com/) and then run `pre-commit install` from the root of the repo.
* There is also a [direnv](https://direnv.net/) `.envrc` file, with a reference to `.envrc.local` for your local settings.  You can use this, for example, to put Go 1.18 in your path if it isn't your system default.

#### Building From Source

* Install needed distro packages
    * Fedora:
      ```
      sudo dnf -y install curl make findutils direnv util-linux-core nodejs gcc
      ```
    * Ubuntu:
      ```
      sudo apt install -y curl make gcc direnv sudo
      curl -fsSL https://deb.nodesource.com/setup_16.x | sudo -E bash -
      sudo apt install -y nodejs
      ```

* Install Go 1.18 or better

  Use `go version` to check your Go version.  If your distro doesn't include at least 1.18, you can install it locally using:
  ```
  mkdir $HOME/go-1.18.3 && \
    curl -L https://go.dev/dl/go1.18.3.linux-amd64.tar.gz | \
    tar xfvz - --strip-components=1 -C $HOME/go-1.18.3
  ```
  If you need to do this, consider also installing [direnv](https://direnv.net/) and creating an `.envrc.local` file containing something like:
  ```
  PATH_add $HOME/go-1.18.3/bin
  ```

* Build the software
  ```
  make all
  ```

