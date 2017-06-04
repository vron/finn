package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/yhat/scrape"
	"golang.org/x/net/html"
)

type Item struct {
	Code int
	Type string
}

func nf(s string) {
	log.Fatalln(s)
}

var parsers = []func(n *html.Node, it *Item) error{
	pType,
}

// Find the type of housing this is
func pType(n *html.Node, it *Item) error {
	// Search for the title
	n, _ = scrape.Find(n, func(node *html.Node) bool {
		return node.Type == html.TextNode && node.Data == "Boligtype"
	})
	fmt.Println(n.Parent.NextSibling)
	log.Fatal("sf")
	if n == nil {
		nf("could not find boligtype")
	}
	n = n.Parent.NextSibling.FirstChild
	if n == nil {
		nf("could not find boligtype value")
	}
	fmt.Println(n)
	log.Fatal("sf")
	it.Type = strings.ToLower(strings.TrimSpace(n.Data))
	return nil
}
