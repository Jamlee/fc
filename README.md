# Fastvpn

fastvpn can be quickly to start and save your money.

[![Build Status](https://travis-ci.org/Jamlee/fastvpn.svg?branch=master)](https://travis-ci.org/Jamlee/fastvpn)
[![CodeFactor](https://www.codefactor.io/repository/github/jamlee/fastvpn/badge)](https://www.codefactor.io/repository/github/jamlee/fastvpn)

## Features

- Paying for ec2 when only you using fastvpn
- encrypted data transfer

## Usage

### 1. config the aws token

create the file `~/.aws/credentials`

```
[default]
aws_access_key_id = AKIAIwe4fQ64OT5G23LN2Q                          # your aws_access_key_id
aws_secret_access_key = o397+vZrTSgVANAq323UkKTp/ckkOKFYQ8nONYQ1E   # your aws_secret_access_key
```

### 2. start the vpn env

run the command `fastvpn start`


## Change Logs

v0.0.1 (2019/03/15)
- add aws support
- it is use thirdparty `vpn` software
- only support on linux platform

