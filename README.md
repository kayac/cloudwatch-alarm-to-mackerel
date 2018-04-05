# Overview

```
 __________________      _____      _____________
|                  |    |     |    |             |
| cloudwatch alarm | => | SNS | => | This Lambda | => [ Mackerel ]
|__________________|    |_____|    |_____________|

```

`cloudwatch-alarm-to-mackerel` function propagates Cloudwatch Alarm alerts to your mackerel.
And we have to use AWS Simple Notification Service to make the lambda work.

# Usage

## git clone

```
git clone git@github.com:kayac/cw-failed-invoke-to-mackerel.git
cd cw-failed-invoke-to-mackerel
```

## create project.json (or function.json)

```
cp project.json.example project.json
```

and please edit for your project.

- environment

variable        | description
--------------- | ----------------------
HOST_ID         | mackerel host id
MACKEREL_APIKEY | mackerel apikey

## apex deploy

```
apex deploy -D
apex deploy
```

You should deploy with '--set' option if you would avoid to include `MACKEREL_APIKEY` into repo.

```
apex deploy --set MACKEREL_APIKEY=xxx-xxxxxx-xxxxxx
```

# How to alert as CRITICAL on mackerel

We can raise a critical alert on mackerel when to set `CRITICAL` to prefix of Cloudwatch Alarm description.
