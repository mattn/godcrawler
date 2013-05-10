package godcrawler

import (
	"bytes"
	"code.google.com/p/go.net/html"
	"code.google.com/p/mahonia"
	"crypto/sha1"
	"database/sql"
	"encoding/xml"
	"fmt"
	"github.com/jteeuwen/go-pkg-rss"
	"io"
	"log"
	"strings"
	"sync"
	"time"
)

type Entry struct {
	Id string
	Link string
	Site string
	Title string
	Content string
	Created string
}

type Outline struct {
	Title    string     `xml:"title,attr,omitempty"`
	XmlURL   string     `xml:"xmlUrl,attr"`
	HtmlURL  string     `xml:"htmlUrl,attr"`
	Outlines []*Outline `xml:"outline"`
}

func htmlWalk(node *html.Node) {
	if node.Type == html.ElementNode {
		if node.Data == "head" || node.Data == "script" || node.Data == "link" || node.Data == "style" {
			node.Parent.RemoveChild(node)
			return
		}
		var attrs []html.Attribute
		for n := 1; n < len(node.Attr); n++ {
			if !strings.HasPrefix(strings.ToLower(node.Attr[n].Key), "on") {
				attrs = append(attrs, node.Attr[n])
			}
		}
		node.Attr = attrs
	}
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		htmlWalk(c)
	}
}

var dateFormats = []string{
	"01.02.06",
	"02 Jan 2006 15:04:05 UT",
	"02 Jan 2006",
	"2 January 2006",
	"2006-01-02 15:04:05 MST",
	"2006-01-02 15:04:05 -0700",
	"2006-01-02",
	"2006-01-02T15:04:05 -0700",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05-0700",
	"2006-01-02T15:04:05-07:00",
	"2006-1-2 15:04:05",
	"2006-1-2",
	"Jan 2, 2006 15:04:05 MST",
	"Jan 2, 2006 3:04:05 PM MST",
	"January 02, 2006 15:04:05 MST",
	"Mon, 02 2006 15:04:05 MST",
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 02 Jan 2006 15:04:05 -0700",
	"Mon, 02 Jan 2006 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04:05 UT",
	"Mon, 02 Jan 2006 15:04:05 Z",
	"Mon, 02 Jan 2006 15:04:05",
	"Mon, 02 Jan 2006",
	"Mon, 02 January 2006",
	"Mon, 2 Jan 2006",
	"Mon, 2 Jan 2006, 15:04 -0700",
	"Mon, 2 January 2006, 15:04 -0700",
	"Monday, 02 January 2006 15:04:05 -0700",
	"Monday, 2 Jan 2006 15:04:05 -0700",
	time.ANSIC,
	time.RubyDate,
	time.UnixDate,
	time.RFC822,
	time.RFC822Z,
	time.RFC850,
	time.RFC1123,
	time.RFC1123Z,
	time.RFC3339,
}

type Crawler struct {
	db    *sql.DB
	feed  *feeder.Feed
	mutex sync.Mutex
}

func (c *Crawler) handleFeed(feed *feeder.Feed, ch *feeder.Channel, items []*feeder.Item) {
	for _, item := range items {
		var r io.Reader
		if item.Content != nil {
			r = strings.NewReader(item.Content.Text)
		} else {
			r = strings.NewReader(item.Description)
		}
		doc, err := html.Parse(r)
		if err != nil {
			log.Println(err)
			continue
		}
		htmlWalk(doc)
		var buf bytes.Buffer
		for doc != nil {
			if doc.Data != "" && doc.Data != "html" && doc.Data != "body" {
				break
			}
			doc = doc.FirstChild
		}
		if doc != nil {
			err = html.Render(&buf, doc)
			if err != nil {
				log.Println(err)
				continue
			}
		}

		guid := item.Guid
		link := ""
		for _, l := range item.Links {
			fmt.Println(l)
			if l.Href != "" && (l.Type == "text/html" || l.Rel == "alternate") {
				link = l.Href
				break
			}
		}
		if link == "" && len(item.Links) == 1 {
			link = item.Links[0].Href
		}
		if link == "" {
			link = guid
		}

		date := time.Now().Format("2006-01-02 15:04:05")
		for _, dateFormat := range dateFormats {
			if t, err := time.Parse(dateFormat, item.PubDate); err == nil {
				date = t.Format("2006-01-02 15:04:05")
				break
			}
		}
		if guid == "" {
			guid = link
		}

		site := ch.Title
		if site == "" {
			site = ch.Description
		}

		if link != "" {
			s := sha1.New()
			s.Write([]byte(guid))
			guid = fmt.Sprintf("%X", s.Sum(nil))

			go func(id, link, site, title, content, created string) {
				c.mutex.Lock()
				println("Stored", link)
				_, err = c.db.Exec("insert into ENTRY(id, url, site, title, content, created) values(?,?,?,?,?,?)",
					id,
					link,
					site,
					title,
					content,
					date,
				)
				if err != nil {
					log.Println(err)
				}
				c.mutex.Unlock()
			}(guid, link, site, item.Title, string(buf.Bytes()), date)
		}
	}
}

func New(db *sql.DB) *Crawler {
	c := &Crawler{
		db: db,
	}
	c.feed = feeder.New(5, true, nil, func(f *feeder.Feed, ch *feeder.Channel, items []*feeder.Item) {
		c.handleFeed(f, ch, items)
	})
	return c
}

func (c *Crawler) Run() {
	for {
		links := []string{}
		c.mutex.Lock()
		rows, err := c.db.Query("select url from FEED")
		if err == nil {
			for rows.Next() {
				var link string
				err = rows.Scan(&link)
				if err == nil {
					links = append(links, link)
				}

			}
			rows.Close()
		}
		c.mutex.Unlock()

		for _, link := range links {
			println("Fetching", link)
			time.Sleep(1 * time.Second)
			err := c.feed.Fetch(link, func(charset string, input io.Reader) (io.Reader, error) {
				return mahonia.NewDecoder(charset).NewReader(input), nil
			})
			if err != nil {
				log.Println(err)
			}
		}
		time.Sleep(3 * time.Minute)
	}
}

type opml struct {
	Id      int64
	Title   string
	XmlURL  string
	HtmlURL string
}

func opmlWalk(r *Outline, callback func(string, string)) {
	if r.XmlURL != "" {
		callback(r.Title, r.XmlURL)
	}
	for _, kid := range r.Outlines {
		opmlWalk(kid, callback)
	}
}

func (c *Crawler) ImportOPML(r io.Reader) error {
	var b struct {
		Body Outline `xml:"body"`
	}
	err := xml.NewDecoder(r).Decode(&b)
	if err != nil {
		return err
	}

	tx, _ := c.db.Begin()

	opmlWalk(&b.Body, func(title, xmlurl string) {
		_, err := tx.Exec("insert into FEED(title, url, created) values(?,?,?)",
			title,
			xmlurl,
			time.Now().Format("2006-01-02 15:04:05"),
		)
		if err != nil {
			log.Println(err)
		}
	})
	tx.Commit()
	return nil
}

func (c *Crawler) Entries(num int) (entries []Entry, err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	rows, err := c.db.Query("select id, url, site, title, created from ENTRY order by created desc limit ?", num)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry Entry
		err := rows.Scan(&entry.Id, &entry.Link, &entry.Site, &entry.Title, &entry.Created)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return
}

func (c *Crawler) Entry(id string) (*Entry, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	row := c.db.QueryRow("select id, url, site, title, content, created from ENTRY where id = ?", id)
	var entry Entry
	err := row.Scan(&entry.Id, &entry.Link, &entry.Site, &entry.Title, &entry.Content, &entry.Created)
	if err != nil {
		return nil, err
	}
	return &entry, nil
}
