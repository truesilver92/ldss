package lib

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

type Book struct {
	base    jsonBook
	catalog *Catalog
	parser  *Book
	parent  Item
	dbCache cache
}

type bookDBConnection struct {
	db                                 *sql.DB
	stmtChildren, stmtUri, stmtContent *sql.Stmt
}

const sqlQueryNode = `
	SELECT
		node.id,
		node.title,
		node.uri,
		CASE WHEN node.content IS NULL THEN 0 ELSE 1 END,
		(SELECT COUNT(*) FROM node subnode WHERE subnode.id = node.id) node_count
	FROM node
`

func newBook() *Book {
	b := &Book{}

	b.dbCache.construct = func() (interface{}, error) {
		var l bookDBConnection
		path := source.BookPath(b)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			fmt.Println("File not found")
			return nil, &NotDownloadedBookErr{err, b}
		}
		db, err := sql.Open("sqlite3", path)
		if err != nil {
			return nil, err
		}
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM node;").Scan(&count)
		if err != nil {
			return nil, &NotDownloadedBookErr{err, b}
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
		l.stmtContent, err = db.Prepare("SELECT content FROM node WHERE id = ?")
		if err != nil {
			return nil, err
		}
		return l, nil
	}
	return b
}

func (b *Book) String() string {
	return fmt.Sprintf("%v {%v}", b.base.Name, b.base.GlURI)
}

func (b *Book) ID() int {
	return b.base.ID
}

func (b *Book) Name() string {
	return b.base.Name
}

func (b *Book) URL() string {
	return b.base.URL
}

func (b *Book) Path() string {
	return b.base.GlURI
}

func (b *Book) Language() *Language {
	return b.catalog.language
}

func (b *Book) Children() ([]Item, error) {
	nodes, err := b.Index()
	if err != nil {
		return nil, err
	}
	items := make([]Item, len(nodes))
	for i, n := range nodes {
		items[i] = n
	}
	return items, nil
}

func (b *Book) Parent() Item {
	return b.parent
}

func (b *Book) db() (*bookDBConnection, error) {
	db, err := b.dbCache.get()
	return db.(*bookDBConnection), err
}

func (l *Book) Index() ([]Node, error) {
	return l.nodeChildren(Node{})
}

func (b *Book) nodeChildren(parent Node) ([]Node, error) {
	l, err := b.db()
	if err != nil {
		return nil, err
	}
	rows, err := l.stmtChildren.Query(parent.id)
	if err != nil {
		return nil, err
	}
	nodes := make([]Node, 0)
	for rows.Next() {
		node := Node{Book: b}
		if node.id > 0 {
			node.parent = node
		}
		err := rows.Scan(&node.id, &node.name, &node.glURI, &node.hasContent, &node.childCount)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (b *Book) GlUri(uri string) (Node, error) {
	node := Node{Book: b}
	l, err := b.db()
	if err != nil {
		return node, err
	}
	err = l.stmtUri.QueryRow(uri).Scan(&node.id, &node.name, &node.glURI, &node.hasContent, &node.childCount)
	return node, err
}

func (b *Book) Content(node Node) (string, error) {
	l, err := b.db()
	if err != nil {
		return "", err
	}
	var content string
	err = l.stmtContent.QueryRow(node.id).Scan(&content)
	return content, err
}
