# IMPLEMENTATION 
![Implementation](/images/Implementation.png)

---


### Details of implementation
The recipe below provides information about 3 main services:
- Frontend service, the FOXDEN front-end web service designed for FOXDEN end-users
  - by default it is deployed at `http://localhost:9000` URL
- Discovery service, the FOXDEN discovery services which provides site-URL
  associations
  - by default it is deployed at `http://localhost:8320` URL
  - so far it only has `/sites` end-point which you may used and it
  provides site information in JSON data-format
- MetaData servuce, the FOXDEN meta-data services which contains meta-data
information about specific sites
  - by default it is deployed at `http://localhost:8300` URL
  - so far it only has `/meta` end-point which you may used and it
  provides meta-data information in JSON data-format

At this moment the Discovery and MetaData services use fake data, i.e.
we hard-coded site and meta-data info. And, frontend service relies on usage of
`play.min.io` to simulate storage access. But it is based on
[min.io Go SDK](https://min.io/docs/minio/linux/developers/go/minio-go.html)
to provide access to the storage.

Here is full set of instructions to run FOXDEN on your local node:
```
# clone three FOXDEN repositories
git clone git@github.com:FOXDEN/Discovery.git
git clone git@github.com:FOXDEN/MetaData.git
git clone git@github.com:FOXDEN/Frontend.git

# compile code in these repositories
# in each repository it will compile `web` executeable
# which represents web server

cd Frontend
make
cd ../MetaData
make
cd ../Discovery
make
cd ../
```

Now, you can use the following script to start all services at once:
```
#!/bin/bash

mkdir -p logs

for srv in Metadata Discovery Frontend
do
    pid=`ps auxww | grep "$srv/web" | grep -v grep | awk '{print $2}'`
    if [ -n "$pid" ]; then
        echo "kill previous $srv/web process $pid"
        kill -9 $pid
    fi
    echo "Start $srv service..."
    nohup ./$srv/web 2>&1 1>& logs/$srv.log < /dev/null & \
        echo $! > logs/$srv.pid
done
```
The script above creates logs area and starts each service independently.
In log area you'll have correspoding log and pid files for your inspection.

Once all services have started we may perform individual tests:

---


### Setup s3 storage at a site.
We can setup s3 storage on a specific site by running on it [minio](https://min.io) server, e.g.
```
# login to Cornell site and start s3 server
minio server /nfs/chess/s3
```
It will provide required URL of the server which now we can inject into
Discovery service

At your site S3 setup you'll be provided RootUser and RootPass parameters,
or they can be fed to minio command. They become yoru access key and
secrets to access s3 storage via API. Therefore, we should encrypt them
to propagate to Discovery service. This can be done via
[enc]() tool as following:
```
# here is ane example how to encrypt entry `test` with secret `bla` and `aes cipher`
./enc -cipher aes -entry test -secret bla -action encrypt
dd15043547b9d422d5859e853a33f71921b9257b2ca181183c6aa99411390a38

# decrypt encrypted hex entry with you cipher and secret
./enc -cipher aes -entry dd15043547b9d422d5859e853a33f71921b9257b2ca181183c6aa99411390a38 -secret bla -action decrypt
test
```

---


### Register new site in Data Discovery service
To register new site in Data Discovery service we should perform
the following set of actions:

- inject site information to Discovery service:
```
curl -X POST -H "Content-type: application/json" \
    -d '{"name":"cornell", "url": "http://localhost:xxxx", "access_key":"xxx", "access_secret":"xxx"}' \
    http://localhost:8320/site
```
Once we inserted the record we may look-up back existing sites in discovery
service
- look-up site information about existing (registered) sites:
```
curl -s http://localhost:8320/sites
[{"name":"cornell","url":"http://localhost:xxx"}]%
```
At this point we have one registered site `cornell` in our discovery
service. This demonstrates how client will interact with Data Discovery
service.

---


### Handling MetaData information
Now, when we have sites some sites available we can inject
some meta-data about our materials. First, let's inject
new meta-data record about our `cornell` site:
```
# inject few records about minearls waste
curl -X POST -H "Content-type: application/json" \
    -d '{"site":"cornell", "description": "mineral waste", "tags": ["waste", "minerals"]}' \
    http://localhost:8300/meta

# you may inject as many records as you like
...
```

Now, we can query MetaData service about existing records, e.g.

- test MetaData service:
```
curl -s http://localhost:8300/meta | jq
[
  {
    "site": "cornell",
    "description": "mineral waste",
    "tags": [
      "waste",
      "minerals"
    ]
  }
]
```

---


### FOXDEN frontend
So far we described how various clients can interact with FOXDEN
services. Assuming that this information will be injected at some
point we can demonstrate how we can navigate it using OreCase frontend
service.

For that let's visit our frontend URL: `http://localhost:9000` and visit
`Sites` page. It will show Sites with corresponding MetaData, and provide
details of specific site and show its data (storage info).

---


### Port allocation
So far we follow these rules, all FOXDEN services should utilize 83xx ports,
e.g.
- 8343 production frontend
- 8344 testbed frontend
- 8300 metadata service
- 8310 provenance service
- 8320 discovery service
- 8330 s3 service
- 8340 data-management service
- 8350 analytics service
- 8380 auth service

