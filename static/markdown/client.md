# Command Line Tool
The official command line tool is called *client*.

```
./client help
client command line tool
Complete documentation at http://www.lepp.cornell.edu/CHESSComputing

Usage:
  client [command]

Available Commands:
  auth        client auth command
  completion  Generate the autocompletion script for the specified shell
  dbs         client dbs command
  help        Help about any command
  meta        client meta command
  s3          client s3 command
  search      client search command

Flags:
      --config string   config file (default is $HOME/.srv.yaml)
  -h, --help            help for client
      --verbose int     verbosity level)

Use "client [command] --help" for more information about a command.
```

### Search for data
To use this tool user must obtain valid kerberos ticket
```
# get kerberos ticket
kinit <user>@CLASSE.CORNELL.EDU

# obtain CHESS Data Management Token
./client auth token /tmp/krb5cc_502

# your token will be as following
eyJh...

# set your CHESS_TOKEN environment
export CHESS_TOKEN=eyJh...
```

Now we can search for some data:
```
# get meta-data records
./client meta ls {}
---
DID     : 1702410249514460928
Schema  : ID3A
Cycle   : 2023-3
Beamline: [3A]
BTR     : 3731-b
....

# get concrete metadata record
./client meta view 1702410249514460928
```

Or, you may use data discoveru search
```
# get all meta-data records
./client search {}
```

### Add new data
```
```
