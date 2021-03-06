package xmlsig

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
)

/* canonicalize produces canonical XML when marshalling the data structure
provided as data. Go's xml encoder generates something that's pretty close,
but it repeats namespace declarations for each element which isn't correct.
It also doesn't sort attribute names.
*/
func canonicalize(data interface{}) ([]byte, string, error) {
	// write the item to a buffer
	var buffer, out bytes.Buffer
	writer := bufio.NewWriter(&buffer)
	encoder := xml.NewEncoder(writer)
	err := encoder.Encode(data)
	if err != nil {
		return nil, "", err
	}
	encoder.Flush()
	// read it back in
	decoder := xml.NewDecoder(bytes.NewReader(buffer.Bytes()))
	namespaces := &stack{}
	outWriter := bufio.NewWriter(&out)
	firstElem := true
	id := ""
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := token.(type) {
		case xml.StartElement:
			// Check the first element for an ID to include in the reference
			if firstElem {
				firstElem = false
				for i := range t.Attr {
					localName := t.Attr[i].Name.Local
					if localName == "ID" || localName == "Id" || strings.HasSuffix(localName, "Id") {
						id = t.Attr[i].Value
					}
				}
			}
			writeStartElement(outWriter, t, namespaces)

		case xml.EndElement:
			namespaces.Pop()
			if !strings.HasPrefix(t.Name.Space, "http") {
				fmt.Fprintf(outWriter, "</%s:%s>", t.Name.Space, t.Name.Local)
			} else {
				fmt.Fprintf(outWriter, "</%s>", t.Name.Local)
			}

		case xml.CharData:
			outWriter.Write(t)
		}
	}
	outWriter.Flush()
	return out.Bytes(), id, nil
}

func writeNameSapce(namespaces *stack, writer io.Writer, start xml.StartElement) {
	currentNs, err := namespaces.Top()
	namespace := start.Name.Space
	if err != nil {
		// No namespaces yet declare ours
		if strings.HasPrefix(namespace, "http") {
			fmt.Fprintf(writer, " %s=\"%s\"", "xmlns", namespace)
		}
	} else {
		// Different namespace declare ours
		if currentNs != namespace {
			if strings.HasPrefix(namespace, "http") {
				fmt.Fprintf(writer, " %s=\"%s\"", "xmlns", namespace)
			}
		}
	}
	namespaces.Push(namespace)
}

func writeStartElement(writer io.Writer, start xml.StartElement, namespaces *stack) {
	if !strings.HasPrefix(start.Name.Space, "http") {
		fmt.Fprintf(writer, "<%s:%s", start.Name.Space, start.Name.Local)
	} else {
		fmt.Fprintf(writer, "<%s", start.Name.Local)
	}
	sort.Sort(canonAtt(start.Attr))

	writeNameSapce(namespaces, writer, start)

	nsmap := make(map[string]string)
	for _, att := range start.Attr {
		// Skip xmlns declarations they're handled above
		if "xmlns" == att.Name.Local {
			continue
		}
		// is this a declaration for an attribute namespace
		if "xmlns" == att.Name.Space {
			fmt.Fprintf(writer, " xmlns:%s=\"%s\"", att.Name.Local, att.Value)
			nsmap[att.Value] = att.Name.Local
			continue
		}
		// is attribute namespaced?
		if att.Name.Space == "" {
			fmt.Fprintf(writer, " %s=\"%s\"", att.Name.Local, att.Value)
		} else {
			fmt.Fprintf(writer, " %s:%s=\"%s\"", nsmap[att.Name.Space], att.Name.Local, att.Value)
		}
	}
	fmt.Fprint(writer, ">")
}

// Attributes must be sorted as part of canonicalization. This type implements sort.Interface for a slice of xml.Attr.
type canonAtt []xml.Attr

// Len is part of sort.Interface.
func (att canonAtt) Len() int {
	return len(att)
}

// Swap is part of sort.Interface.
func (att canonAtt) Swap(i, j int) {
	att[i], att[j] = att[j], att[i]
}

// Less is part of sort.Interface.
func (att canonAtt) Less(i, j int) bool {
	iName := att[i].Name
	jName := att[j].Name
	// xmlns without prefix goes first
	if iName.Local == "xmlns" {
		return true
	}
	if jName.Local == "xmlns" {
		return false
	}
	// namespace declarations go next sorted by prefix
	if iName.Space == "xmlns" {
		if jName.Space != "xmlns" {
			return true
		}
		return iName.Local < jName.Local
	}
	if jName.Space == "xmlns" {
		// we know iName Space isn't xmlns
		return false
	}
	// Lastly sort by attribute name namespace first
	if iName.Space != jName.Space {
		return iName.Space < jName.Space
	}
	return iName.Local < jName.Local
}
