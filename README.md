# Turning on/off docker containers on demand
OnDocker is a simple reverse proxy which can start and stop one or multiple containers based on the request activity to a pre-defined URL. This is specially useful for containers which are not used very often but consume a lot of CPU/RAM on idle. Simply opening a URL will turn on the container and redirect the traffic automatically to the respective backend. There is no need to manually start/stop containers to save resources.

----------
## Features
- Turn on/off multiple related containers at once (e.g. frontend and database containers)
- Set an inactivity timeout (optional)
- Set a sleep time interval during which the container is stopped and cannot be started (optional)
- Customizable error and loading pages


## config.json parameter description

| Name    | Type    | Default    |  Description    |
|---------------- | --------------- | --------------- | --------------- |
| `containerName`    | string    | **Required**    | Name of docker container. E.g. "photoprism"    |
| `helperContainers`    | arry of strings    | optional    | Additional docker containers which will be started/stopped together with the main container. E.g. ["photoprism_mariadb"]    |
| `hosts`    | array of strings    | **Required**   | Array of all inbound request addresses for one docker container. E.g. ["http://192.168.0.10:3333"]    |
| `backend`    | string    | **Required**    | Backend address of respective docker container. E.g. "http://172.23.0.4:4444"    |
| `maxRetries`    | int    | optional. *Default: 5*   | Maximum number of requests to successfully reach a container before showing the error page. E.g. 5   |
| `inactivityTimeout`    | int    | optional. *Default: 15*    | Time after which an inactive docker container is turned off. Setting this to 0 disables this feature.    |
| `sleepStartTime`    | string    | optional    | Beginning of sleep period during which the docker container is stopped and cannot be started. E.g. "23:00"    |
| `sleepStopTime`    | string    | optional    | End of sleep period during which the docker container is stopped and cannot be started. E.g. "07:00"    |

## config.json example
```json [
    {
        "containerName": "wikijs",
        "hosts": ["http://192.168.0.10:10000", "https://wikijs.mydomain.com"],
        "backend": "http://172.23.0.129:3000",
        "maxRetries":3,
        "inactivityTimeout": 15
    },
    {
        "containerName": "photoprism",
        "helperContainers": ["photoprism_mariadb"],
        "hosts": ["https://photoprism.mydomain.com"],
        "backend": "http://172.23.0.121:2342",
        "maxRetries":5,
        "inactivityTimeout": 0,
        "sleepStartTime": "18:00",
        "sleepStopTime": "09:00"
    }
]

```

> In the first example the container wikijs will start if there is a request to either "http://192.168.0.10:10000" or "https://wikijs.mydomain.com". Traffic coming to those hosts will be redirected to "http://172.23.0.129:3000". 3 attempts to reach the backend will be made before showing an error page. After 15 minutes without any traffic to either host addresses the container will be turned off.

>In the second example the container photoprism and its helper container photoprism_mariadb will start if there is a request to "https://photoprism.mydomain.com". 5 attempts to reach the backend will be made before showing an error page. There is no inactivity timeout but the container will be turned off at 18:00 and can be started after 09:00 everyday. 

## Error and loading pages
errorPage.html and loadingPage.html can be customized as needed. Do not remove the tags between double {{}} as those will be used to provide information about the container.

Available parameters are:
- `ContainerName` (name of container)
- `Timeout` (inactivity timeout)
- `CurrentRetries` (current number of retries)
- `MaxRetries` (Max number of retries)

## Usage
Default port used is 10000.
If config.json or html pages are missing then they will be created automatically when running in a docker container.

### Docker-compose -> recommended :star:
```yaml
version: "3.6"
services:
  ondocker:
    image: leonardopc/ondocker
    container_name: ondocker
    restart: unless-stopped
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Europe/Berlin
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /path/to/data:/config
    ports:
      - 10000:10000
```
### Docker cli
```console
docker run -d \
  --name=ondocker \
  -e PUID=1000 \
  -e PGID=1000 \
  -e TZ=Europe/Berlin \
  -p 10000:10000 \
  -v /path/to/data:/config \
  -v /var/run/docker.sock:/var/run/docker.sock:ro
  --restart unless-stopped \
  leonardopc/ondocker:latest
```


### Building locally
```bash
git clone https://github.com/leonardopc/ondocker.git
mkdir /config
cd ondocker
cp -a config/. /config
cd src
go build
./OnDocker
```
