# taildog

taildog is a CLI tool to query and tail Datadog logs like `tail -f`. 

# Installation

You can download the executable binary from the [releases page](https://github.com/kamatama41/taildog/releases).

```console
$ latest=$(curl -s https://api.github.com/repos/kamatama41/taildog/releases/latest | jq -r ".name")
$ os=$(uname)
$ curl -LO https://github.com/kamatama41/taildog/releases/download/${latest}/taildog_${latest:1}_${os}_amd64.zip
$ unzip taildog_${latest:1}_${os}_amd64.zip && rm taildog_${latest:1}_${os}_amd64.zip
```

# Usage

Before running, you must set the two environment variables `DD_API_KEY` and `DD_APP_KEY`, which can be taken on the [Datadog APIs page](https://app.datadoghq.com/account/settings#api).

## Examples

#### Follow all logs

```console
$ taildog
```

#### Follow logs with a query

```console
$ taildog -q "service:my-app"
```

#### Query logs for a duration (without following, max 1000 logs)

```console
$ taildog -q "service:my-app" --from 2019-07-10T11:00:00Z --to 2019-07-10T11:00:05Z
```

#### Show logs with a custom format

```console
# Customize header
$ taildog -q "service:my-app" -h "{{.Timestamp}}: "

# Customize message
$ taildog -q "service:my-app" -m "{{.Attributes.my_message}}"
```

# Note

taildog uses the [Log Query API](https://docs.datadoghq.com/api/?lang=bash#get-a-list-of-logs) of Datadog which is rate limited, `300` per hour per organization, and taildog calls the API every 15 seconds by default (240 times per hour). So you might encounter the rate limit error when using taildog with multiple users in your organization. 
