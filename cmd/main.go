package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/juju/errgo"
	"github.com/yhat/scrape"
)

var (
	fData          string
	fGetObj        int
	fGetObjRefresh bool
	fList          bool
	fImage         bool
	fDownloadList  bool
	fParse         int
)

func init() {
	flag.StringVar(&fData, "data", "./data", "Data directory where all crawled information should be stored")
	flag.IntVar(&fGetObj, "get", 0, "The finn code to get and store locally in the data cache, nothing is done if it exisits ")
	flag.BoolVar(&fGetObjRefresh, "r", false, "True if a refresh should be forced even if it does exist in the cache")

	flag.BoolVar(&fImage, "image", false, "Download images associated with each house")

	flag.BoolVar(&fList, "list", false, "True if all sold items should be listed")
	flag.BoolVar(&fDownloadList, "download", false, "True if all sold items should be downloaded")

	flag.IntVar(&fParse, "parse", 0, "Parse the specific (or all) id and print the object to stdout")
}

func main() {
	flag.Parse()

	// First step is to see if anything should be downloaded explicitly
	if fGetObj > 0 {
		e := getObj(fGetObj, fGetObjRefresh)
		hErr(e)
	}

	// Check if items should be listed and downloaded
	if fList {
		list, e := listSoldObj()
		hErr(e)
		for i, v := range list {
			if fDownloadList {
				e := getObj(v, fGetObjRefresh)
				hErr(e)
			}
			fmt.Println(i, v)

		}
	}
	if fParse != 0 {
		ids := []int{}
		if fParse == -1 {
			ids = getCached()
		} else {
			ids = append(ids, fParse)
		}
		// Parse the data that we want out of the article
		for _, v := range ids {
			it, e := parseItem(v)
			hErr(e)
			fmt.Println(it)
		}
	}
}

type rep struct {
	r io.Reader
}

func (r rep) Read(p []byte) (n int, err error) {
	n, e := r.r.Read(p)
	if e != nil {
		return n, e
	}
	bytes.Replace(p, []byte{'\r'}, []byte{' '}, -1)
	bytes.Replace(p, []byte{'\n'}, []byte{' '}, -1)
	return n, e
}

func parseItem(code int) (Item, error) {
	// Start by reading the entire to memory so we can mess about as we want
	f, e := os.Open(filepath.Join(fData, "cache", strconv.Itoa(code), "index.html"))
	defer f.Close()
	if e != nil {
		return Item{}, errgo.Mask(e)
	}

	// Apply each of the parsers
	root, err := html.Parse(rep{f})
	f.Close()
	if err != nil {
		return Item{}, errgo.Mask(err)
	}
	it := Item{}
	for _, p := range parsers {
		e := p(root, &it)
		hErr(e)
	}
	return it, nil
}

// Return an array of all ids we have cached
func getCached() []int {
	fis, e := ioutil.ReadDir(filepath.Join(fData, "cache"))
	if e != nil {
		log.Fatalln(e)
	}
	r := make([]int, 0, len(fis))
	for _, v := range fis {
		n, e := strconv.Atoi(v.Name())
		if e != nil {
			log.Fatalln(e)
		}
		r = append(r, n)
	}
	return r
}

func hErr(e error) {
	if e != nil {
		log.Fatalln(e)
	}
}

func folder(code int) string {
	return filepath.Join(fData, "cache", strconv.Itoa(code))
}

// Use finn to list all sold appartments
func listSoldObj() ([]int, error) {
	v := url.Values{}
	v.Add("updateCoords", "updateCoords")
	v.Add("layers", "10002")
	v.Add("mapType", "normap")
	v.Add("h", "912")
	v.Add("w", "1183")
	v.Add("heightx", "912")
	v.Add("widthx", "1183")
	v.Add("x", "591.5")
	v.Add("cy", "456")
	v.Add("scalex", "205424.10263878873")
	v.Add("level", "11.925")
	v.Add("ads", "false")
	v.Add("datetime", "1492632031160")
	v.Add("touch", "0")
	v.Add("minX", "1162400.6830265")
	v.Add("minY", "8360525.2180038")
	v.Add("maxX", "1211004.1229177")
	v.Add("maxY", "8397994.649264")
	v.Add("proj", "EPSG:3857")
	v.Add("autoLimit", "true")
	v.Add("activetab", "iad")
	v.Add("searchKey", "search_id_realestate_homes_sold")
	v.Add("showSold", "true")
	v.Add("showActive", "false")
	v.Add("responseType", "json")
	resp, e := http.PostForm("https://kart.finn.no/ajax.jsf", v)
	if e != nil {
		return nil, errgo.Mask(e)
	}

	// Try to parse the response as JSON and get the items we should have
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	type Poi struct {
		Ids []string
	}
	type Item struct {
		NumberOfPois int
		Pois         map[string]Poi
	}
	it := Item{}
	e = dec.Decode(&it)
	if e != nil {
		return nil, errgo.Mask(e)
	}
	// Loop through all the ids
	ids := make([]int, 0, it.NumberOfPois)
	idx := make(map[int]bool)
	for _, v := range it.Pois {
		for _, vv := range v.Ids {
			no, _ := strconv.Atoi(vv)
			if _, ok := idx[no]; ok {
				continue
			}
			idx[no] = true
			ids = append(ids, no)
		}
	}
	return ids, nil
}

func getObj(code int, force bool) error {
	// First check if it is in the cache, if so do not do anything unless
	// force is set
	f := folder(code)
	fi, e := os.Stat(f)
	if e != nil {
		if !os.IsNotExist(e) {
			return errgo.Mask(e)
		}
		ee := os.MkdirAll(f, 0777)
		if ee != nil {
			return errgo.Mask(ee)
		}
	}
	if fi != nil && !fi.IsDir() {
		return errgo.New("cache item should be a folder: " + f)
	}
	if e == nil && !force {
		log.Println("ignoring item allready in cache: " + strconv.Itoa(code))
		return nil
	}

	// So we should download the item from finn, togeather with all the links we
	// need which in particular is all the images
	url := "https://www.finn.no/realestate/homes/ad.html?finnkode=" +
		strconv.Itoa(code)
	response, err := http.Get(url)
	if err != nil {
		return errgo.New("Error while downloading: " + strconv.Itoa(code) + ", " + err.Error())
	}
	defer response.Body.Close()

	output, err := os.Create(filepath.Join(f, "index.html"))
	if err != nil {
		return errgo.Mask(err)
	}
	defer output.Close()
	io.Copy(output, response.Body)
	output.Close()
	response.Body.Close()

	file, e := os.Open(filepath.Join(f, "index.html"))
	if e != nil {
		return errgo.Mask(e)
	}
	defer file.Close()
	imgs, e := findImages(file)
	file.Close()
	if e != nil {
		return errgo.Mask(e)
	}
	if len(imgs) <= 0 {
		log.Println("found no images for: " + strconv.Itoa(code))
	}

	if fImage {
		for i, v := range imgs {
			// Download all the images and save them as 0.jpg, 1.jpg etc
			// so that they are available
			url := v
			response, err := http.Get(url)
			if err != nil {
				return errgo.New("Error while downloading: " + strconv.Itoa(code) +
					", " + err.Error())
			}
			defer response.Body.Close()
			output, err := os.Create(filepath.Join(f, strconv.Itoa(i)+".jpg"))
			if err != nil {
				return errgo.Mask(err)
			}
			defer output.Close()
			io.Copy(output, response.Body)
			output.Close()
			response.Body.Close()
		}
	}

	// Sleep a random time between 0 and 3s between each to decrease the load from us a bit
	to := time.Duration(rand.Intn(3000)) * time.Millisecond
	fmt.Println("sleeping for: ", to)
	time.Sleep(to)

	return nil
}

// From an index html document find all the links
// to the images that are associated with the ad
func findImages(r io.Reader) ([]string, error) {
	root, err := html.Parse(r)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	// Search for the title
	par, ok := scrape.Find(root, func(node *html.Node) bool {
		if len(node.Attr) > 0 && node.Attr[0].Key == "data-carousel-container" {
			return true
		}
		return false
	})

	if ok {
		imgs := []string{}
		// Loop through the images and get them all
		for _, v := range scrape.FindAllNested(par, scrape.ByTag(atom.Img)) {
			// Find the src attribute and store that
			for _, a := range v.Attr {
				if a.Key == "src" || a.Key == "data-src" {
					imgs = append(imgs, a.Val)
				}

			}
		}
		return imgs, nil
	}
	return []string{}, nil
}
