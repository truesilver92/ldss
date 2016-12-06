package lib

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"os"
	"sort"
	"strconv"
	"sync"
	"unicode"
	"unicode/utf8"

	_ "github.com/mattn/go-sqlite3"
)

// Represents a book in the catalog or one of it's folders and
// provides a way to access the nodes in it's database if
// downloaded.
type Book struct {
	base    *jsonBook
	catalog *Catalog
	parent  Item
	dbCache cache
}

type bookDBConnection struct {
	db                                                        *sql.DB
	stmtChildren, stmtUri, stmtId, stmtContent, stmtFootnotes *sql.Stmt
}

var BookConnectionLimit = 10
var bookConnectionLock sync.Mutex
var bookConnections []*Book

func connectBookDB(book *Book) {
	bookConnectionLock.Lock()
	defer bookConnectionLock.Unlock()

	if bookConnections == nil {
		bookConnections = make([]*Book, BookConnectionLimit)
	}
	for _, x := range bookConnections {
		if x == book {
			return
		}
	}
	log.Print(len(bookConnections), cap(bookConnections))
	if len(bookConnections) < cap(bookConnections) {
		log.Println("added", book.Name())
		bookConnections[len(bookConnections)] = book
	} else {
		log.Println("replaced", book.Name())
		x := bookConnections[0]
		if x != nil {
			log.Println("closed", x.Name())
			x.dbCache.Close()
		}
		bookConnections[0] = book
		if x == book {
			return
		}
	}
}

const sqlQueryNode = `
	SELECT
		node.id,
		node.title,
		node.uri,
		node.parent_id,
		CASE WHEN node.content IS NULL THEN 0 ELSE 1 END,
		(SELECT COUNT(*) FROM node subnode WHERE subnode.id = node.id) node_count
	FROM node
`

const sqlQueryRef = `
	SELECT
		ref.ref_name,
		ref.link_name,
		ref.ref
	FROM ref
	WHERE
		ref.node_id = ?
`

func newBook(base *jsonBook, catalog *Catalog, parent Item) *Book {
	b := &Book{}
	b.base = base
	b.catalog = catalog
	b.parent = parent

	b.dbCache.construct = func() (interface{}, error) {
		var l bookDBConnection
		path := bookPath(b)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			dlErr := notDownloadedBookErr{book: b}
			dlErr.err = err
			return nil, dlErr
		}
		db, err := sql.Open("sqlite3", path)
		if err != nil {
			return nil, err
		}
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM node;").Scan(&count)
		if err != nil {
			dlErr := notDownloadedBookErr{book: b}
			dlErr.err = err
			return nil, dlErr
		}
		l.db = db
		l.stmtChildren, err = db.Prepare(sqlQueryNode + " WHERE parent_id = ?")
		if err != nil {
			return nil, err
		}
		l.stmtUri, err = db.Prepare(sqlQueryNode + " WHERE uri = ?")
		if err != nil {
			return nil, err
		}
		l.stmtId, err = db.Prepare(sqlQueryNode + " WHERE id = ?")
		if err != nil {
			return nil, err
		}
		l.stmtContent, err = db.Prepare("SELECT content FROM node WHERE id = ?")
		if err != nil {
			return nil, err
		}
		l.stmtFootnotes, err = db.Prepare(sqlQueryRef)
		if err != nil {
			return nil, err
		}
		return &l, nil
	}
	return b
}

// A short human-readable representation of the book, mostly useful for debugging.
func (b *Book) String() string {
	return fmt.Sprintf("%v {%v}", b.base.Name, b.base.GlURI)
}

// An ID for the book. This is unique to this book within it's language.
func (b *Book) ID() int {
	return b.base.ID
}

// The name of this book.
func (b *Book) Name() string {
	return b.base.Name
}

// The URL the database of this book is located at online.
func (b *Book) URL() string {
	return b.base.URL
}

// The Gospel Library Path of this book, unique within it's language
func (b *Book) Path() string {
	return b.base.GlURI
}

// The language this book is in
func (b *Book) Language() *Language {
	return b.catalog.language
}

// Children in this book. This is identical to the Index function, but returns
// the index as a []Item
func (b *Book) Children() ([]Item, error) {
	nodes, err := b.Index()
	if err != nil {
		return nil, err
	}
	items := make([]Item, len(nodes))
	for i, n := range nodes {
		if subnodes, err := n.Children(); err == nil && len(subnodes) == 1 {
			items[i] = subnodes[0]
		} else {
			items[i] = n
		}
	}
	return items, nil
}

// Parent Folder or Catalog of this book
func (b *Book) Parent() Item {
	return b.parent
}

// Next Book in the Folder
func (b *Book) Next() Item {
	return genericNextPrevious(b, 1)
}

// Previous Book in the Folder
func (b *Book) Previous() Item {
	return genericNextPrevious(b, -1)
}

// The SQLite database connector with some prepared statements. Cached for subsequent uses.
func (b *Book) db() (*bookDBConnection, error) {
	connectBookDB(b)

	db, err := b.dbCache.get()
	if err != nil {
		return nil, err
	}
	return db.(*bookDBConnection), nil
}

// Returns the Nodes at the root of this book.
func (l *Book) Index() ([]*Node, error) {
	return l.nodeChildren(nil)
}

// Returns the Nodes that are children of the passed node. If the passed Node
// is nil, it will return the Index
func (b *Book) nodeChildren(parent *Node) ([]*Node, error) {
	l, err := b.db()
	parentId := 0
	if err != nil {
		return nil, err
	}
	if parent != nil {
		parentId = parent.id
	}
	rows, err := l.stmtChildren.Query(parentId)
	if err != nil {
		return nil, err
	}
	nodes := make([]*Node, 0)
	for rows.Next() {
		node := &Node{Book: b}
		err := rows.Scan(&node.id, &node.name, &node.path, &node.parentId, &node.hasContent, &node.childCount)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (b *Book) lookupId(id int) (*Node, error) {
	node := &Node{Book: b}
	l, err := b.db()
	if err != nil {
		return node, err
	}
	err = l.stmtId.QueryRow(id).Scan(&node.id, &node.name, &node.path, &node.parentId, &node.hasContent, &node.childCount)
	return node, err

}

func (b *Book) lookupPath(uri string) (*Node, error) {
	node := &Node{Book: b}
	l, err := b.db()
	if err != nil {
		return node, err
	}
	err = l.stmtUri.QueryRow(uri).Scan(&node.id, &node.name, &node.path, &node.parentId, &node.hasContent, &node.childCount)
	if err != nil {
		return nil, fmt.Errorf("Path %v not found", uri)
	}
	return node, err
}

func (b *Book) nodeContent(node *Node) (string, error) {
	l, err := b.db()
	if err != nil {
		return "", err
	}
	var content string
	err = l.stmtContent.QueryRow(node.id).Scan(&content)
	return content, err
}

func (b *Book) nodeFootnotes(node *Node, verses []int) ([]Footnote, error) {
	l, err := b.db()
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, fmt.Errorf("Cannot use nil Node in nodeFootnotes()")
	}
	rows, err := l.stmtFootnotes.Query(node.id)
	if err != nil {
		return nil, err
	}
	refs := make([]Footnote, 0)
	for rows.Next() {
		ref := Footnote{}
		var content string
		err = rows.Scan(&ref.Name, &ref.LinkName, &content)
		ref.Content = template.HTML(content)
		ref.item = node
		if err != nil {
			return nil, err
		}
		if len(verses) > 0 {
			verseNumString := ref.Name
			if char, length := utf8.DecodeLastRuneInString(verseNumString); unicode.IsDigit(char) {
				verseNumString = verseNumString[:len(verseNumString)-length-1]
			}
			if verseNum, err := strconv.Atoi(verseNumString); err != nil {
				if sort.SearchInts(verses, verseNum) > -1 {
					refs = append(refs, ref)
				}
			}
		} else {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}
