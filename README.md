# fastvpn

fastvpn is quickly to start for use and no use no paid for `aws`.

[![Build Status](https://travis-ci.org/Jamlee/fastvpn.svg?branch=master)](https://travis-ci.org/Jamlee/fastvpn)
[![CodeFactor](https://www.codefactor.io/repository/github/jamlee/fastvpn/badge)](https://www.codefactor.io/repository/github/jamlee/fastvpn)

With fastvpn, paying for aws when you using vpn, it is extraly save your money for securely network.

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

