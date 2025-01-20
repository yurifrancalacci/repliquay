# Repliquay

## Overview

**Repliquay** is a tool written in go used to configure organizations, repositories, permissions, robots etc. etc... on one or more Quay Enterprise instances. These configurations are written in one or more YAML formatted files.

It uses standard Quay APIs to configure the instance. APIs are accessed via preconfigured OAuth token defined in specific configuration file.

Repliquay can also be used to `clone` a Quay instance to one or more other instances. As, up to today, there is no way to use standard APIs to set robot passwords, created robots have random password.

## Usage
```
repliquay --help
Usage of repliquay:
  -clone
        clone first quay configuration to others. Requires >= 2 quays (ignore all other options)
  -conf string
        repliquay config file (override all opts) (default "/repos/repliquay.conf")
  -debug
        print debug messages (default false)
  -dryrun
        enable dry run (default false)
  -insecure
        disable TLS connection (default false)
  -ldapsync
        enable ldap sync (default false)
  -quaysfile string
        quay token file name
  -repo value
        quay repo file name
  -retries int
        max retries on api call failure (default 3)
  -skipVerify
        enable/disable TLS validation
  -sleep int
        sleep length ms when reaching max connection (default 100)

```

Options:

- ``clone`` enable cloning functionality and requires 2 or more instances defined
- ``conf`` could be use to store repliquay parameters instead of use command line options
- ``debug`` print additional logging lines
- ``dryrun`` do not perform any http call
- ``insecure`` use clear HTTP protocol and not HTTPS
- ``ldapsync`` enable Quay API call to configure LDAP sync in teams definition
- ``quaysfile`` containg Quay instance definitions (host/api token/max connections)
- ``repo`` contains repository definitions. Could be specified one or more times (e.g. --repo=file1.yaml --repo=file2.yaml)
- ``retries`` maximum number of HTTP retries in case of HTTP error code >= 5xx
- ``skipVerify`` do not perform TLS certificate validation
- ``sleep`` milliseconds to wait before trying an HTTP call when Quay instance is handling more connection than max value specified on the configuration file


## Create Oauth API token

To create an OAuth access token so you can access the API for your organization:

- Log in to Red Hat Quay and select your Organization (or create a new one).
- Select the Applications icon from the left navigation.
- Select Create New Application and give the new application a name when prompted.
- Select the new application.
- Select Generate Token from the left navigation.
- Select the checkboxes to set the scope of the token and select Generate Access Token.
- Review the permissions you are allowing and select Authorize Application to approve it.
- Copy the newly generated token to use to access the API.
