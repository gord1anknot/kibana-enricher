// es-enricher enriches Logstash formatted Elasticsearch documents with additional context asynchronously as a batch job
package main

/*
   Copyright 2015 Brad Rowe
   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at
       http://www.apache.org/licenses/LICENSE-2.0
   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

import (
    . "github.com/mattbaird/elastigo/lib"
    "time"
    "log"
    "flag"
    "fmt"
    "os"
    "encoding/json"
    "text/template"
	"bytes"
)

type CorrelationId struct {
    Name  string
    Value string
}

var (
    // Connection parameters
    indexName      string
    typeName       string
    port 	       string
    host           string
    // CLI argument parameters
    updateDocument string
    idField        string
    idValue        string
)

func init() {
    defaultIndexName := fmt.Sprintf("logstash-%s", time.Now().Format("2006.01.02"))
    localHostname, err		 := os.Hostname()

    if err != nil {
        localHostname = "localhost"
    }

	flag.StringVar(
		&idField,
		"idField",
		"correlation.id",
		"the name of the field that contains the correlation ID",
	)
	flag.StringVar(
		&idValue,
		"idValue",
		"",
		"the value of the id field - ALL documents matching will be updated",
	)
    flag.StringVar(
        &updateDocument,
        "json",
        "{}",
        "json document to update documents the correlation ID, default is no-op",
    )
    flag.StringVar(
        &indexName,
        "esindex",
        defaultIndexName,
        "elasticsearch document type")
    flag.StringVar(
        &typeName,
        "estype",
        "audit_log",
        "elasticsearch document type")
    flag.StringVar(
        &host,
        "eshost",
        localHostname,
        "elasticsearch hostname, the result of $(hostname -f) or similar by default")
    flag.StringVar(
        &port,
        "port",
        "9200",
        "elasticsearch HTTP API port")
}

func main() {
    flag.Parse()

    parsedDoc := parseUpdateDoc(updateDocument)

	conn := connect()

    indexer := conn.NewBulkIndexerErrors(10, 60)
    indexer.Start()
    defer indexer.Stop()
    defer log.Println("Done.")

    tmplParams := CorrelationId{Name: idField, Value: idValue}
    tmpl, err := template.New("esquery").Parse(qryTmpl)
    if err != nil { panic(err) }
	buf := new(bytes.Buffer)
	err = tmpl.Execute(buf, tmplParams)
	if err != nil { panic(err) }
	qry := buf.String()

    result, err := conn.Search(indexName, typeName, nil, qry)

    if err != nil {
        log.Fatalf("Error when searching for documents! %s", err)
    }

    log.Printf("Found %d log events match the query.\n", result.Hits.Total)
    log.Printf("Elasticsearch reports the query took %d seconds to execute.\n", result.Took)
    if result.Hits.Len() == 0 {
        log.Printf("0 hits found on index %s with type %s. Nothing more to do.\n", indexName, typeName)
    } else {
        log.Printf("Got the Document ID(s) for %d Hits, queueing document ID(s) in memory for enrichment...\n", result.Hits.Len())
        rawText, err := result.Hits.Hits[0].Source.MarshalJSON()
        if err == nil {
            fmt.Println(string(rawText))
        }


        for _, elem := range result.Hits.Hits {

            indexer.UpdateWithPartialDoc(
                elem.Index,
                elem.Type,
                elem.Id,
                "",
                nil,
                parsedDoc,
                false,
                true)
        }
        log.Println("Done queueing events for batch update, please wait...")

        // This bit starts the bulk indexing client.
        // So here's the deal: Flushing does seem to work, you just have to give the
        // elastigo bulk indexer's channel a moment to get on the scheduler and receive the *initial*
        // message... after that, Stop() and Flush() work as advertised.
        time.After(time.Millisecond * 20)
    }
}

func connect() *Conn {
	conn := NewConn()
	conn.SetHosts(
		[]string{
			host,
		})
	conn.SetPort(port)

	_, err := conn.ExistsIndex(indexName, typeName, nil)

	if err != nil {
		fmt.Printf(
			"Enrichment failed. Elasticseach index %s or document type %s not found at host %s on port %s.\n\n",
			indexName,
			typeName,
			host,
			port)
		fmt.Println("Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	return conn
}

func parseUpdateDoc(updateDocument string) interface{} {
	var parsedDoc interface{}
	b := []byte(updateDocument)
	updateDocErr := json.Unmarshal(b, &parsedDoc)

	if updateDocErr  != nil {
		log.Fatal("Enrichment failed. Unable to parse the update document into well formed json.")
	}
	return parsedDoc
}

const qryTmpl = `{
    "size": 100,
    "query" : {
        "filtered": {
            "filter": {
                "and" : [
                    {
                        "term": {
                            "{{.Name}}": "{{.Value}}"
                        }
                    }
                ]
            }
        }
    }}`