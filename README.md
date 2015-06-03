# Logstash Enrichment

This project enriches Logstash formatted Elasticsearch documents with additional context asynchronously as a batch job, using the [Elasticsearch update document API](https://www.elastic.co/guide/en/elasticsearch/reference/1.4/docs-update.html).

Because denormalized data is better for Kibana 3 searches.

## Use Case

This project helps prevent the following situation:

You have some web service that takes HTTP requests from a client and creates logs that looks like this:

`{ "type": "REPORT-REQUEST", "report.type": "Cat Videos", "id": "DEADBEEF-1234-XC15-8341-BEEF9D4BB6D2", "source.ip": "10.210.100.90"}`

`{ "type": "REPORT-REQUEST", "report.type": "Tweets", "id": "AFFCEABA-4950-4C15-8341-8E959D4BB6D2",  "source.ip": "10.210.100.93"}`

You have a different web service that asynchronously creates the requests report. It generates logs that look like this:

`{ "type": "REPORT-CREATED", "report.type": "Tweets", "id": "AFFCEABA-4950-4C15-8341-8E959D4BB6D2", "time.taken": "992000", "size": "390k"}`

`{ "type": "REPORT-CREATED", "report.type": "Cat Videos", "id": "DEADBEEF-1234-XC15-8341-BEEF9D4BB6D2", "time.taken": "25393020", "size": "204930k"}`

Your pager goes off at 3:00 AM - Zabbix has told you that long transaction times are causing a service to crash. Thanks to a very
simple search inside Kibana, you already know several of the `report.type`s that appear in the problem requests, but
you still need to figure out which single `source.ip`, if any, is responsible for the huge `time.taken` values that are crashing
your server so that you filter that source out.

Kibana 3 has a `top N` query, so you can search for the `top N` `source.ip` addresses on all documents with no issue.
However, the documents with `type:REPORT-CREATED` are the only ones with the value you care about - `time.taken`.
`REPORT-CREATED` documents will not show up in any query using `source.ip` in a filter or facet, and a Kibana 3 `top N`
query isn't helpful either. You need the aggregation query API to figure out which `source.ip` is submitting report requests
that generate the huge reports. Unfortunately you don't know anything about Elasticsearch, and the guy that does is on vacation... you only know how to use Kibana 3.

Instead of that mess, this project can run as a cron job all the time like this:

```
./kibana-enricher \
    -idField="id" \
    -idValue="AFFCEABA-4950-4C15-8341-8E959D4BB6D2"
    -json='{"report.type":"Cat Videos", "source.ip": "10.210.100.93"}'
```

Now all your `type:REPORT-CREATED` documents have a `"source.ip"` field. Now, no power in the 'verse can stop you. Barring hardware failure of course but let's not jinx it.

## Usage



## Build and Deployment

Install dependencies with `go get`:

```
go get github.com/mattbaird/elastigo
```

Alternatively, if you have many go-lang projects you can use vendorized dependencies courtesy [`gvp`](https://github.com/pote/gvp) and [`gpm`](https://github.com/pote/gpm#go-package-manager-):

OSX Installation of `gpm` and `gvp`:

```
brew update && brew install gpm gvp
```

Usage:

```
source gvp
gpm install
```

After that, build the project:

```
go build .
```

Deploy the binary `kibana-enricher` as you see fit. When using Ubuntu on AWS, I will do something like this:

```
scp kibana-enricher ubuntu@my-deployment-target:/home/ubuntu/
ssh ubuntu@my-deployment-target -- sudo mv /home/ubuntu/kibana-enricher /usr/local/bin/
```

Run the binary on a regular schedule using `cron` or similar.