## Channel Stats Slack Bot
A slack bot to collect sentiment analysis and other statistics for a channel.

## API Documentation
The bot stores event counts by hour such that when querying for results all
calls must include a `start-hour` and an `end-hour`. If no **start** or
 **end** hour is provided then stats for the last 7 days is returned for
the specified channel.

The following is a list of available counter types for use with the
 `<type>` parameter.

Type       | Description
-----------|------------
messages   | The number of messages seen in channel
positive   | The number of messages that had positive sentiment seen in channel
negative   | The number of messages that had negative sentiment seen in channel
link       | The number of messages that contain an http link
emoji      | The number of messages that contain an emoji
word-count | The number of words counted in the channel

**NOTE: All date / hour formats follow RFC3339 short format `2018-12-06T01`**

### Retrieve total counts for a stat over a span of hours
Calls to `/sum` retrieve a summation of all counters for a specified duration

```
GET /api/channels/<channel-name>/sum/<type>
```

Parameter   | Description
------------|------------
start-hour  | The start hour
end-hour    | Then end hour

##### Examples
Get a count of messages for the last 7 days for channel 'general'
```bash
$ curl http://localhost:2020/api/channels/general/sum/messages | jq
{
    "start-hour": "2018-12-06T18",
    "end-hour": "2018-12-13T18",
    "items":[
        {
            "User": "foo",
            "Sum": 20
        },
        {
            "User": "bar",
            "Sum": 2
        }
    ]
}
```
Get a count of negative sentiment messages since midnight
```bash
$ curl http://localhost:2020/api/channels/general/sum/negative?start-date=2018-12-13T00
```

Get a count of messages with emoji's in the last 2 days
```bash
$ curl http://localhost:2020/api/channels/general/sum/emoji?start-hour=2018-12-11T00&end-hour=2018-12-13T00
```

### Retrieve raw counter data
You can get access to the raw counter data via the `/data` endpoint

```
GET /api/channels/<channel-name>/data/<type>
```

Parameter   | Description
------------|------------
start-hour  | The start hour
end-hour    | Then end hour

##### Examples
Get all the counters for the last 7 days for the `general` channel
```bash
$ curl http://localhost:2020/api/channels/general/data/messages | jq
{
    "start-hour": "2018-12-06T19",
    "end-hour": "2018-12-13T19",
    "items":[
        {
            "Hour": "2018-12-06T21",
            "UserID": "U02C73W94",
            "UserName": "redbo",
            "ChannelID": "C02C073ND",
            "ChannelName": "general",
            "DataType": "messages",
            "Value": 10
        },
        {
            "Hour": "2018-12-06T21",
            "UserID": "U02CG0QLN",
            "UserName": "glange",
            "ChannelID": "C02C073ND",
            "ChannelName": "general",
            "DataType": "messages",
            "Value": 8
        }
    ]
}
```

## Web UI
This project doesnt have a UI yet... but if anyone is interested in making a cool looking interface the
place holder for a UI is available via http://localhost:2020/ui/index.html

## Run via docker
Easiest way to get started is by running `channel-stats` in a docker
container using the pre-built image.

```bash
# Download the docker-compose file
$ curl -O https://raw.githubusercontent.com/thrawn01/channel-stats/master/docker-compose.yaml

# Edit the compose file to provide your slack bot token
# environment variables
$ vi docker-compose.yaml

# Run the docker container
$ docker-compose up -d

# Hit the API at localhost:2020
$ curl http://localhost:2020/api | jq
```

## Run locally
* Download the release binary from `FIXME`
* Download the example yaml file
* Edit the example file to provide your slack bot token
* Run `./channel-stats --config /path/to/config.yaml`
* Hit the api at localhost:2020 `curl http://localhost:2020/api | jq`

## Develop
channel-stats uses go 1.11 modules for dependency management

```bash
# Clone the repo
$ git clone FIXME
$ make
```


