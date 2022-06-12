![Connectopus](./docs/logo.png)

### What is this

### What can it do

### How to build it

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
  ```
  mkdir $HOME/go-1.18.3 && \
    curl -L https://go.dev/dl/go1.18.3.linux-amd64.tar.gz | \
    tar xfvz - --strip-components=1 -C $HOME/go-1.18.3
  ```

* Add direnv hook to the shell and authorize the directory.    
  This will put Go 1.18 in your path whenever you cd into this directory.  If you use a shell other than bash, consult https://direnv.net/docs/hook.html.
  ```
  echo PATH_add $HOME/go-1.18.3/bin > .envrc.local
  echo 'eval "$(direnv hook bash)"' >> $HOME/.bashrc
  . $HOME/.bashrc
  direnv allow
  ```

* Build the software
  ```
  make all
  ```

### How to run it once built
