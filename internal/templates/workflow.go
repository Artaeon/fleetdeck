package templates

// SharedWorkflow builds without distributing a root-equivalent Docker/SSH key.
// Deployments remain an explicit action by the trusted FleetDeck administrator.
const SharedWorkflow = `name: Build
on:
  push:
    branches: [main, master]
  pull_request:
permissions:
  contents: read
jobs:
  build:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803 # v6
      - name: Build images
        run: docker compose -f docker-compose.yml build
`
