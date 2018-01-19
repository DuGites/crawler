package main

import (
	"fmt"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"

	"github.com/codegangsta/cli"
	pb "github.com/mendoncangelo/crawler/crawler"
)

const (
	address = "localhost:40052"
)

func startCrawler(client pb.CrawlerClient, link *pb.LinkRequest) {
	resp, err := client.Crawl(context.Background(), link)
	if err != nil {
		log.Fatalf("Could not start Crawler %v", err)
	}
	log.Printf(resp.Message)
}

func stopCrawler(client pb.CrawlerClient, stop *pb.StopRequest) {
	resp, err := client.Stop(context.Background(), stop)
	if err != nil {
		log.Fatalf("Could not stop Crawler %v", err)
	}
	log.Printf(resp.Message)
}

func listSiteTree(client pb.CrawlerClient, list *pb.ListRequest) {
	stream, err := client.ListVisitedUrls(context.Background(), list)
	if err != nil {
		log.Fatalf("Could not List Site Tree %v", err)
	}
	for {
		// Receiving the stream of data
		link, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("%v.ListVisitedUrls(_) = _, %v", client, err)
		}
		log.Println("Crawled Urls:", link)
	}
}

func getFullURL(link string) string {
	_, err := http.Get("https://" + link)
	if err != nil {
		return "https://" + link
	}
	_, err = http.Get("http://" + link)
	if err != nil {
		return "http://" + link
	}
	return "https://" + link
}

func main() {

	// Set up a connection to the gRPC server.
	var link string
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Connection to gRPC was not successfull: %v", err)
	}
	defer conn.Close()

	app := cli.NewApp()
	app.Usage = "Site Crawler"
	app.Commands = []cli.Command{
		{
			Name:  "start",
			Usage: "Start crawling a website value= https://www.lipsum.com",
			Action: func(c *cli.Context) error {
				if c.NArg() > 1 {
					return cli.NewExitError("Please provide only one url", 1)
				}
				link = getFullURL(c.Args().First())
				_, err := url.ParseRequestURI(link)
				if err != nil {
					return cli.NewExitError("Please provide a correct url", 1)
				}
				client := pb.NewCrawlerClient(conn)
				link := &pb.LinkRequest{Url: link}
				startCrawler(client, link)
				return nil
			},
		},
		{
			Name:  "stop",
			Usage: "Stop crawling the current website",
			Action: func(c *cli.Context) error {
				if c.NArg() > 1 {
					return cli.NewExitError("Please provide only one url to stop", 1)
				}
				link = getFullURL(c.Args().First())
				_, err := url.ParseRequestURI(link)
				if err != nil {
					return cli.NewExitError("Please provide a correct url", 1)
				}
				client := pb.NewCrawlerClient(conn)
				stop := &pb.StopRequest{Link: link}
				stopCrawler(client, stop)
				return nil
			},
		},
		{
			Name:  "list",
			Usage: "List the crawled urls",
			Action: func(c *cli.Context) error {
				if c.NArg() > 0 {
					return cli.NewExitError("This command does not take any input", 1)
				}
				client := pb.NewCrawlerClient(conn)
				list := &pb.ListRequest{List: true}
				fmt.Println("Listing Site Tree")
				listSiteTree(client, list)
				return nil
			},
		},
	}
	sort.Sort(cli.CommandsByName(app.Commands))

	app.Run(os.Args)

}
