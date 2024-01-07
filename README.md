# brain server

Simple server for the tasks/notes app brain.

Currently supports basic CRUD operations for tasks.

## Local dev setup

```sh
# Copy .env file containing for local dev auth credentials
cp env.template .env

# Set up local cert
mkcert localhost
mkcert -install

# Run the server
go run *.go
```
