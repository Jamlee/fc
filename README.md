# fastvpn

fastvpn is a quickly vpn installer base on aws ec2.

[![Build Status](https://travis-ci.org/Jamlee/fastvpn.svg?branch=master)](https://travis-ci.org/Jamlee/fastvpn)
[![CodeFactor](https://www.codefactor.io/repository/github/jamlee/fastvpn/badge)](https://www.codefactor.io/repository/github/jamlee/fastvpn)

## Usage

### Step 1

create the file `~/.aws/credentials`

```
[default]
aws_access_key_id = AKIAIwe4fQ64OT5G23LN2Q                          # your aws_access_key_id
aws_secret_access_key = o397+vZrTSgVANAq323UkKTp/ckkOKFYQ8nONYQ1E   # your aws_secret_access_key
```

### Step 2

run the command `fastvpn start`

```
NAME:
   fastvpn - creating a vpn server fastly

USAGE:
   fastvpn [global options] command [command options] [arguments...]

VERSION:
   0.0.0

COMMANDS:
     start    start the fastvpn instance
     status   get running vm status
     stop     stop running vm
     help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h     show help
   --version, -v  print the version
```
