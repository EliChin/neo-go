# NeoGo consensus node

NeoGo node can act as a consensus node. A consensus node differs from regular
one in that it participates in block acceptance process using dBFT
protocol. Any committee node can also be elected as CN therefore they're
expected to follow the same setup process as CNs (to be ready to become CNs
if/when they're elected).

While regular nodes on Neo network don't need any special keys CNs always have
one used to sign dBFT messages and blocks. So the main difference between
regular node and consensus/committee node is that it should be configured to
use some key from some wallet.

## Running a CN on public networks

### Hardware requirements

While NeoGo can be very conservative with its resource consumption, public
network CN provides some service to the general audience and thus should have
enough hardware resources to do its job reliably. We recommend amd64 machine
with at least two cores, 8+ GB of memory and 64 GB SSD (disk space
requirements depend on actual chain height and
KeepOnlyLatestState/RemoveUntraceableBlocks settings, 64 GB is considered to
be enough for the first year of blockchain).

### OS requirements

NeoGo is a single binary that can be run on any modern GNU/Linux
distribution. We recommend using major well-supported OSes like CentOS, Debian
or Ubuntu, make sure they're updated with the latest security patches.

No additional packages are needed for NeoGo CN.

### Installation

Download NeoGo binary [from
Github](https://github.com/nspcc-dev/neo-go/releases) or use [Docker
image](https://hub.docker.com/r/nspccdev/neo-go). It has everything included,
no additional plugins needed.

Take appropriate (mainnet/testnet) configuration [from the
repository](https://github.com/nspcc-dev/neo-go/tree/master/config) and save
in some directory (we'll assume that it's available in the same directory as
neo-go binary).

### Configuration and execution

Add the following subsection to `ApplicationConfiguration` section of your
configuration file (`protocol.mainnet.yml` or `protocol.testnet.yml`):
```
  UnlockWallet:
    Path: "wallet.json"
    Password: "welcometotherealworld"
```
where `wallet.json` is a path to your NEP-6 wallet and `welcometotherealworld`
is a password to your CN key. Run the node in a regular way after that:

```
$ neo-go node --mainnet --config-path ./
```
where `--mainnet` is your network (can be `--testnet` for testnet) and
`--config-path` is a path to configuration file directory. If the node starts
fine it'll be logging events like synchronized blocks. The node doesn't have
any interactive CLI, it only outputs logs so you can wrap this command in a
systemd service file to run automatically on system startup.

Notice that the default configuration has RPC and Prometheus services enabled,
you can turn them off for security purposes or restrict access to them with a
firewall. Carefuly review all other configuration options to see if they meet
your expectations. Details on various configuration options are provided in the
[node configuration documentation](node-configuration.md), CLI commands are
provided in the [CLI documentation](cli.md).

### Registration

To register as a candidate use neo-go as CLI command with an external RPC
server for it to connect to (for chain data and transaction submission). You
can use any public RPC server or an RPC server of your own like the node
started at previous step. We'll assume that you're running the next command on
the same node in default configuration with RPC interface available at port
10332.

Candidate registration is performed via NEO contract invocation that costs
1000 GAS, so your account must have enough of it to pay. You need to provide
your wallet and address to neo-go command:
```
$ neo-go wallet candidate register -a NiKEkwz6i9q6gqfCizztDoHQh9r9BtdCNa -w wallet.json -r http://localhost:10332
```
where `NiKEkwz6i9q6gqfCizztDoHQh9r9BtdCNa` is your address, `wallet.json` is a
path to NEP-6 wallet file and `http://localhost:10332` is an RPC node to
use.

This command will create and send appropriate transaction to the network and
you should then wait for it to settle in a block. If all goes well it'll end
with "HALT" state and your registration will be completed. You can use
`query tx` command to see transaction status or `query candidates` to see if
your candidate was added.

### Voting

After registration completion if you own some NEO you can also vote for your
candidate to help it become CN and receive additional voter GAS. To do that
you need to know the public key of your candidate, which can either be seen in
`query candidates` command output or extracted from wallet `wallet dump-keys`
command:

```
$ neo-go wallet dump-keys -w wallet.json
NiKEkwz6i9q6gqfCizztDoHQh9r9BtdCNa (simple signature contract):
0363f6678ea4c59e292175c67e2b75c9ba7bb73e47cd97cdf5abaf45de157133f5
```

`0363f6678ea4c59e292175c67e2b75c9ba7bb73e47cd97cdf5abaf45de157133f5` is a
public key for `NiKEkwz6i9q6gqfCizztDoHQh9r9BtdCNa` address. To vote for it
use:
```
$ neo-go wallet candidate vote -a NiKEkwz6i9q6gqfCizztDoHQh9r9BtdCNa -w wallet.json -r http://localhost:10332 -c 0363f6678ea4c59e292175c67e2b75c9ba7bb73e47cd97cdf5abaf45de157133f5

```
where `NiKEkwz6i9q6gqfCizztDoHQh9r9BtdCNa` is voter's address, `wallet.json`
is NEP-6 wallet file path, `http://localhost:10332` is RPC node address and
`0363f6678ea4c59e292175c67e2b75c9ba7bb73e47cd97cdf5abaf45de157133f5` is a
public key voter votes for. This command also returns transaction hash and you
need to wait for this transaction to be accepted into one of subsequent blocks.

## Private NeoGo network
### Using existing Dockerfile

neo-go comes with two preconfigured private network setups, the first one has
four consensus nodes and the second one uses single node. Nodes are packed
into Docker containers and four-node setup shares a volume for chain data.

Four-node setup uses ports 20333-20336 for P2P communication and ports
30333-30336 for RPC (Prometheus monitoring is also available at ports
20001-20004). Single-node is on ports 20333/30333/20001 for
P2P/RPC/Prometheus.

NeoGo default privnet configuration is made to work with four node consensus,
you have to modify it if you're to use single consensus node.

Node wallets are located in the `.docker/wallets` directory where
`wallet1_solo.json` is used for single-node setup and all the other ones for
four-node setup.

#### Prerequisites
- `docker` of version >= 20.10.0
- `docker-compose`
- `go` compiler

#### Instructions
You can use existing docker-compose file located in `.docker/docker-compose.yml`:
```bash
make env_image # build image
make env_up    # start containers, use "make env_single" for single CN
```
To monitor logs:
```bash
docker-compose -f .docker/docker-compose.yml logs -f
```

To stop:
```bash
make env_down
```

To remove old blockchain state:
```bash
make env_clean
``` 

### Start nodes manually
1. Create a separate config directory for every node and
place corresponding config named `protocol.privnet.yml` there.

2. Edit configuration file for every node.
Examples can be found at `config/protocol.privnet.docker.one.yml` (`two`, `three` etc.).
    1. Add `UnlockWallet` section with `Path` and `Password` strings for NEP-6
       wallet path and password for the account to be used for consensus node.
    2. Make sure that your `MinPeers` setting is equal to
       the number of nodes participating in consensus.
       This requirement is needed for nodes to correctly
       start and can be weakened in future.
    3. Set `Address`, `Port` and `RPC.Port` to the appropriate values.
       They must differ between nodes.
    4. If you start binary from the same directory, you will probably want to change
       `DataDirectoryPath` from the `LevelDBOptions`. 

3. Start all nodes with `neo-go node --config-path <dir-from-step-2>`.
