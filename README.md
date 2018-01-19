## Multiple Site crawler
  The application consists of implementing a "Web Crawler as a gRPC service". It consists of a command line client and a local service which runs the actual web crawling. The communication between client and server is defined as a gRPC service. The crawler only follows links on the
  domain of the provided URL and not any external links. It uses channels and goroutines for enhanced performance.

## Installation

  Standard `go get`:

  ```
  $ go get github.com/mendoncangelo/crawler
  ```
  Make sure you have go installed and the GOPATH set. 
  
## Usage 
  To start the crawler server.
  ```
  go run server.go
  ```
  Will start crawling a specific site.
  ```
  go run client.go start url_name
  e.g. go run client.go start www.hashicorp.com
  ```
  Will list the urls crawled so far.
  ```
  go run client.go list
  ```

To stop a specific site crawler.
  ```
  e.g. go run client.go stop www.hashicorp.com
  ```

  You can also start other crawlers for different websites while others sites are being crawled.
  ```
  e.g. go run client.go start www.nodejs.org
  ```

  You can stop one of the crawlers and have the other one running.
  ```
  e.g. go run client.go stop www.hashicorp.com
  ```

## Enhancements
  Some of the parsing of links needs to be improved. Already visited URLs need to be ignored. 