# Requirements

- [buf](https://buf.build/docs/installation) - a CLI tool to manage your protos.
- [go](https://go.dev/doc/install) - a go module is created to host the go proto client
- [npm](https://docs.npmjs.com/downloading-and-installing-node-js-and-npm) - an npm package is created to host the javscript proto client

# Generate protos

Make changes to protos defined in the `protos` directory and run the script. This will generate the latest protos and write them to the gen folder. This must be done or the regenerate ci job will fail.

```
  make generate
```
