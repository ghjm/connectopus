## Contributing to Connectopus

#### Licensing

Connectopus is currently licensed under the Affero GPL v3.  If you are interested in contributing, I am asking you to agree to a [Contributor License Agreement](https://gist.github.com/ghjm/359bf26d060e5fd6845ec09f224c9231).  This accomplishes two things for me:

* Perhaps in the future I might want to change to a less restrictive open source license.
* If someone is interested in making commercial use of Connectopus in a way the AGPL doesn't permit, I want to maintain the ability to issue a license exception.

The first time you open a pull request, the CLA Assistant at https://cla-assistant.io will ask you to electronically consent to the agreement.  (Thank you to SAP for providing the community with this useful tool.)

#### Development Environment

* My development environment is Fedora.  Ubuntu should also be fine.  CI builds are done on Windows and MacOS.
* I use the JetBrains GoLand IDE, but nothing in the project should be tied to it.
* The project is set up with pre-commit hooks to run local checks when you create a commit.  To use this, install [pre-commit](https://pre-commit.com/) and then run `pre-commit install` from the root of the repo.
* There is also a [direnv](https://direnv.net/) `.envrc` file, with a reference to `.envrc.local` for your local settings.

#### Building From Source

* Make sure you have Go 1.24 or better, Node 22 or better, make, jq and find

* Build the software
  ```
  make all
  ```

