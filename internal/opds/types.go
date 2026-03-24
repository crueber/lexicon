package opds

import "encoding/xml"

// Feed is the root Atom feed element for OPDS catalogs.
type Feed struct {
	XMLName   xml.Name `xml:"feed"`
	XMLNS     string   `xml:"xmlns,attr"`
	XMLNSDC   string   `xml:"xmlns:dc,attr"`
	XMLNSOPDS string   `xml:"xmlns:opds,attr"`
	XMLNSThr  string   `xml:"xmlns:thr,attr,omitempty"`
	ID        string   `xml:"id"`
	Title     string   `xml:"title"`
	Updated   string   `xml:"updated"`
	Links     []Link   `xml:"link"`
	Entries   []Entry  `xml:"entry"`
}

// Entry is a single item in an OPDS feed.
type Entry struct {
	ID      string   `xml:"id"`
	Title   string   `xml:"title"`
	Updated string   `xml:"updated"`
	Authors []Author `xml:"author,omitempty"`
	Summary string   `xml:"summary,omitempty"`
	Content string   `xml:"content,omitempty"`
	Links   []Link   `xml:"link"`
}

// Author represents a book author in an OPDS entry.
type Author struct {
	Name string `xml:"name"`
}

// Link is an Atom link element used for navigation and acquisition.
type Link struct {
	Rel   string `xml:"rel,attr,omitempty"`
	Href  string `xml:"href,attr"`
	Type  string `xml:"type,attr,omitempty"`
	Title string `xml:"title,attr,omitempty"`
	Count string `xml:"thr:count,attr,omitempty"`
}

// MIME type constants for OPDS link relations.
const (
	// typeNavigation is the MIME type for OPDS navigation feeds.
	typeNavigation = "application/atom+xml;profile=opds-catalog;kind=navigation"
	// typeAcquisition is the MIME type for OPDS acquisition feeds.
	typeAcquisition = "application/atom+xml;profile=opds-catalog;kind=acquisition"
	// typeAtom is the generic Atom feed MIME type.
	typeAtom = "application/atom+xml"
)

// opdsContentTypes maps book file formats to their MIME types for acquisition links.
var opdsContentTypes = map[string]string{
	"EPUB": "application/epub+zip",
	"PDF":  "application/pdf",
	"CBZ":  "application/x-cbz",
	"CBR":  "application/x-cbr",
	"MOBI": "application/x-mobipocket-ebook",
	"M4B":  "audio/mp4",
	"MP3":  "audio/mpeg",
	"M4A":  "audio/mp4",
	"OPUS": "audio/ogg",
}
