# sqlbuild

This repo builds the `hslaw/mssql-build` docker image, a tool for creating `microsoft/mssql-server-linux` images with data baked in.

The difficulty in creating these images is that all of your scripts are written in sql, but sql server needs to be running for you to execute them. That means that a `RUN` line in your Dockerfile has to somehow start sql server, run your scripts, and then shutdown sql server.

That's what this package does. The `sqlbuild` binary starts sql server, runs the passed scripts, and then shuts down sql server. The `hslaw/mssql-build` image is just stock `microsoft/mssql-server-linux` with the `sqlbuild` binary added in.

## Setup

To build, you need [docker](https://www.docker.com/) installed. The `microsoft/mssql-server-linux` will fail with the default docker memory limits, so you need to bump them above 3250MB before getting started.

Usage generally looks like this:

```Dockerfile
FROM hslaw/mssql-build
ENV ACCEPT_EULA=Y SA_PASSWORD=!SQLBUILD2018
COPY *.sql /migrations/
RUN sqlbuild exec /migrations
```

That copies your sql scripts into the `/migrations` folder and calls sqlbuild to execute them. Files are executed in lexical order. Configuration is passed using the same environment variables that the `microsoft/mssql-server-linux` image expects. The sqlserver password is set as the value of SA_PASSWORD the first time sqlbuild is run.

Willing to answer any questions. This is early stage software.

## Development

The project comes with a docker-compose file that makes getting into a development environment as easy as `docker-compose run --rm dev`. The first time will take a while as the image needs to build.
