# Garden Linux

A Linux backend for [Garden](https://github.com/cloudfoundry-incubator/garden).

You can deploy Garden using the [Garden BOSH Release repository](https://github.com/cloudfoundry-incubator/garden-linux-release).

Restructure in progress: the old content is moving to the `old/` directory after which the new content will replace it gradually.

See the [old README](old/README.md) for old documentation, caveat lector.

##Testing

To test under `fly`, see [Concourse](https://github.com/concourse/concourse) for set-up, and run

```bash
scripts/garden-fly
```

in the repository root.

`garden-fly` provides the necessary parameters to `fly` which uses `build.yml`
and runs `scripts/concourse-test` on an existing Concourse instance which must
already be running locally in a virtual machine.
