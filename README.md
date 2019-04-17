# Challengize exporter

This exporter exposes statistics from a Challengize (https://www.challengize.com) challenge as Prometheus metrics.

## Getting started

Login to your account on https://www.challengize.com and make sure to click the checkbox "Remember me". Then grab the values of the cookies `REMEMBER` and `JSESSIONID`. These need to be supplied to the exporter as environment variables.

```
REMEMBER="<guid>" JSESSIONID="<sessionid>" ./challengize_exporter
```