# fastvpn

fastvpn is a quickly vpn installer base on aws ec2.

[![Build Status](https://travis-ci.org/Jamlee/fastvpn.svg?branch=master)](https://travis-ci.org/Jamlee/fastvpn)
[![CodeFactor](https://www.codefactor.io/repository/github/jamlee/fastvpn/badge)](https://www.codefactor.io/repository/github/jamlee/fastvpn)

## Usage

### config the aws token

create the file `~/.aws/credentials`

```
[default]
aws_access_key_id = AKIAIwe4fQ64OT5G23LN2Q                          # your aws_access_key_id
aws_secret_access_key = o397+vZrTSgVANAq323UkKTp/ckkOKFYQ8nONYQ1E   # your aws_secret_access_key
```

### start the vpn env

run the command `fastvpn start`


