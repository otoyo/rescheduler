# Rescheduler

Search schedules by keyword on Slack and propose dates for rescheduling.

![sample mov](https://user-images.githubusercontent.com/1063435/50733194-83939a80-11cc-11e9-8684-3f69d9d49fe9.gif)

## Usage

To run this bot, you need to set the following env vars,

```
export SLACK_BOT_ID=xxx
export SLACK_BOT_TOKEN=xoxb-xxxx
export SLACK_VERIFICATION_TOKEN=xxx
export SLACK_USER_ID=xxx # yours
export GAROON_SUBDOMAIN=xxx # xxx.cybozu.com
export GAROON_USER=xxx
export GAROON_PASSWORD=xxx
export GAROON_EXCLUDING_FACILITY_CODE=0601 # optional
```

```
$ dep ensure
$ go build -o bot && ./bot
```
