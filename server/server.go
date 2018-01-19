package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	pb "github.com/mendoncangelo/crawler/crawler"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type server struct {
	spiderPtr *crawlerDS
}

type linkIndex struct {
	index int
	url   string
}

const (
	port               = ":40052"
	concurrentRequests = 50
)

type crawlerDS struct {
	// Complete list of URLS visited.
	visitedUrls map[string]bool
	// URLS that were crawled
	finishedUrls chan string
	// Urls currently waiting to be crawled
	waitingUrls chan linkIndex
	// URL to Index Mapper
	siteIndexURL map[int]string
	// URL to Index Mapper
	siteURLIndex map[string]linkIndex
	// Crawler Specific channel terminator
	terminate map[int]*chan int
	// Number of sites being crawled
	siteIndex int
}

var (
	wg                          sync.WaitGroup
	mapLock                     = sync.RWMutex{}
	newsites                    = make(chan string, 1)
	shutdownSpecificSiteCrawler = make(chan linkIndex, 1)
	httpLimitChannel            = make(chan struct{}, concurrentRequests)
)

// Method to find if uri has scheme and extract the hostname
func (c *crawlerDS) parseURL(uri string) (string, bool) {
	pg, err := url.Parse(uri)
	if err != nil {
		log.Fatal(err)
	}
	if pg.Scheme == "https" || pg.Scheme == "http" {
		return pg.Host, true
	}
	return "", false
}

// Method that processes the href's in the crawled page
func (c *crawlerDS) parseLinks(data *string, pageURL string,
	index int, shutdown chan int) {

	defer wg.Done()
	u, err := url.Parse(pageURL)
	if err != nil {
		log.Fatal(err)
	}
	re := regexp.MustCompile("href=\"(.*?)\"")
	subre := regexp.MustCompile("\"/[\\w]+")

	matchLink := re.FindAllStringSubmatch(string(*data), -1)
	for _, lnk := range matchLink {

		if subre.MatchString(lnk[0]) && lnk[1] != pageURL {
			url := pageURL + lnk[1]
			_, scheme := c.parseURL(url)

			if scheme {
				select {
				case <-shutdown:
					return
				default:
					c.waitingUrls <- linkIndex{index: index, url: url}
				}
			}

		} else if strings.Contains(lnk[1], u.Hostname()) {
			host, scheme := c.parseURL(lnk[1])
			init, _ := url.Parse(pageURL)

			if scheme && host != init.Host {
				select {
				case <-shutdown:
					return
				default:
					c.waitingUrls <- linkIndex{index: index, url: lnk[1]}
				}
			}
		}
	}
}

// Crawler initiator
func (c *crawlerDS) crawler(url string, index int,
	shutdown chan int) {

	defer wg.Done()
	// This will limit the number of HTTP requests.
	httpLimitChannel <- struct{}{}
	resp, err := http.Get(url)
	<-httpLimitChannel

	if err != nil {
		log.Println(err)
	} else {
		select {
		case <-shutdown:
			return
		default:
			// Push the crawled url onto the finishedURL channel
			c.finishedUrls <- url
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Println(err)
			}
			msg := string(body)
			wg.Add(1)
			c.parseLinks(&msg, url, index, shutdown)
		}
	}
}

// Method to print the site tree.
func (c *crawlerDS) listURLS() []string {
	mapLock.RLock()
	defer mapLock.RUnlock()
	links := make([]string, 0)
	for k := range c.visitedUrls {
		links = append(links, k)
	}
	return links
}

func (c *crawlerDS) startCrawling() {

	for {
		select {

		case finURL := <-c.finishedUrls:
			fmt.Println("Crawled URL", finURL)
			mapLock.Lock()
			c.visitedUrls[finURL] = true
			mapLock.Unlock()

		case obj := <-c.waitingUrls:
			wg.Add(1)
			if value, ok := c.terminate[obj.index]; ok {
				go c.crawler(obj.url, obj.index, *value)
			}

		case url := <-newsites:
			fmt.Println("Received new site to crawl", len(newsites),
				url, c.siteIndex)
			// Every site should have a shutdown channel
			shutdown := make(chan int, 1)
			mapLock.Lock()
			c.terminate[c.siteIndex] = &shutdown
			c.siteURLIndex[url] = linkIndex{index: c.siteIndex, url: url}
			c.siteIndexURL[c.siteIndex] = url
			mapLock.Unlock()

			wg.Add(1)
			go c.crawler(c.siteIndexURL[c.siteIndex], c.siteIndex, shutdown)
			c.siteIndex++

		case obj := <-shutdownSpecificSiteCrawler:
			mapLock.RLock()
			log.Println("Stopping Crawler for site", obj.url, obj.index,
				c.terminate[obj.index])
			close(*c.terminate[obj.index])
			delete(c.siteURLIndex, obj.url)
			delete(c.siteIndexURL, obj.index)
			delete(c.terminate, obj.index)
			mapLock.RUnlock()
			log.Println("Cleaning Crawler completed")

		default:
			time.Sleep(5 * time.Second)
			continue
		}
	}
}

// Crawl method implementation for the gRPC
func (s *server) Crawl(ctx context.Context, in *pb.LinkRequest) (*pb.CrawlerResponse, error) {
	mapLock.RLock()
	// given a URL checks to see if its currently being crawled
	_, exists := s.spiderPtr.siteURLIndex[in.Url]
	mapLock.RUnlock()
	if exists {
		msg := fmt.Sprintf("Site %s is already being crawled", in.Url)
		return &pb.CrawlerResponse{Message: msg}, nil
	}
	// put new site on channel
	newsites <- in.Url
	return &pb.CrawlerResponse{Message: "Crawler started crawling"}, nil
}

// Stop method implementation for the gRPC
func (s *server) Stop(ctx context.Context, in *pb.StopRequest) (*pb.StopResponse, error) {
	mapLock.RLock()
	// given a URL checks to see if its currently not being crawled
	indx, exists := s.spiderPtr.siteURLIndex[in.Link]
	mapLock.RUnlock()
	if !exists {
		msg := fmt.Sprintf("Site %s is not being crawled", in.Link)
		return &pb.StopResponse{Message: msg}, nil
	}
	shutdownSpecificSiteCrawler <- linkIndex{url: in.Link, index: indx.index}

	return &pb.StopResponse{Message: "Crawler Stopping"}, nil
}

// List Visited URLS implementation for the gRPC
func (s *server) ListVisitedUrls(in *pb.ListRequest, stream pb.Crawler_ListVisitedUrlsServer) error {

	for _, link := range s.spiderPtr.listURLS() {
		ln := &pb.LinkRequest{Url: link}
		if err := stream.Send(ln); err != nil {
			return err
		}
	}
	return nil
}

func newCrawler() crawlerDS {
	return crawlerDS{
		visitedUrls:  make(map[string]bool),
		siteURLIndex: make(map[string]linkIndex),
		siteIndex:    0,
		finishedUrls: make(chan string),
		siteIndexURL: make(map[int]string),
		waitingUrls:  make(chan linkIndex),
		terminate:    make(map[int]*chan int)}
}

// gRPC main server process
func main() {
	listener, err := net.Listen("tcp", port)
	if err != nil {
		log.Panicf("Failed to start gRPC server: %v", err)
	}

	// Creates a new gRPC server
	srvr := grpc.NewServer()
	ds := newCrawler()

	wg.Add(1)
	go ds.startCrawling()

	pb.RegisterCrawlerServer(srvr, &server{spiderPtr: &ds})
	srvr.Serve(listener)
}
