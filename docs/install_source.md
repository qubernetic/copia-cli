# Installation from source

1. Install [Docker](https://docs.docker.com/get-docker/) and [just](https://just.systems/)

2. Clone this repository

   ```bash
   $ git clone https://github.com/qubernetic/copia-cli.git
   $ cd copia-cli
   ```

3. Build inside the container

   ```bash
   $ just setup
   $ just dev
   $ just build
   $ sudo cp bin/copia-cli /usr/local/bin/
   ```

4. Verify installation

   ```bash
   $ copia-cli --version
   ```

Alternatively, if you have Go 1.26+ installed locally:

```bash
$ go build -o bin/copia-cli ./cmd/copia-cli
```
