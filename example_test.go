package badgerx_test

import (
	"errors"
	"fmt"
	"log"

	badger "github.com/dgraph-io/badger/v4"
	badgerx "github.com/somak2kai/badgerx"
)

type User struct {
	Name string
	Age  int
}

func openDB() *badger.DB {
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true).WithLogger(nil))
	if err != nil {
		log.Fatal(err)
	}
	return db
}

// ExampleNewBadgerXDb demonstrates creating a BadgerXDb with default settings
// (gob encoding, no compression) and performing a basic store and retrieve.
func ExampleNewBadgerXDb() {
	db := openDB()

	xdb := badgerx.NewBadgerXDb(db)
	defer xdb.Close()

	_ = xdb.Update([]byte("user:1"), User{Name: "somak", Age: 30})

	var u User
	_ = xdb.View([]byte("user:1"), &u)

	fmt.Println(u.Name, u.Age)
	// Output: somak 30
}

// ExampleBadgerXDb_Update demonstrates storing a value under a key.
func ExampleBadgerXDb_Update() {
	xdb := badgerx.NewBadgerXDb(openDB())
	defer xdb.Close()

	err := xdb.Update([]byte("user:1"), User{Name: "somak", Age: 30})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("stored")
	// Output: stored
}

// ExampleBadgerXDb_View demonstrates retrieving a value by key.
// Returns badger.ErrKeyNotFound when the key does not exist.
func ExampleBadgerXDb_View() {
	xdb := badgerx.NewBadgerXDb(openDB())
	defer xdb.Close()

	_ = xdb.Update([]byte("user:1"), User{Name: "somak", Age: 30})

	var u User
	err := xdb.View([]byte("user:1"), &u)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(u.Name, u.Age)
	// Output: somak 30
}

// ExampleBadgerXDb_View_notFound demonstrates handling a missing key.
func ExampleBadgerXDb_View_notFound() {
	xdb := badgerx.NewBadgerXDb(openDB())
	defer xdb.Close()

	var u User
	err := xdb.View([]byte("user:missing"), &u)
	if errors.Is(err, badger.ErrKeyNotFound) {
		fmt.Println("not found")
	}
	// Output: not found
}

// ExampleNewZstdCompressor demonstrates using Zstandard compression
// alongside the default gob encoder.
func ExampleNewZstdCompressor() {
	zstdC, err := badgerx.NewZstdCompressor()
	if err != nil {
		log.Fatal(err)
	}

	xdb := badgerx.NewBadgerXDb(openDB(), badgerx.WithCompressor(zstdC))
	defer xdb.Close()

	_ = xdb.Update([]byte("user:1"), User{Name: "somak", Age: 30})

	var u User
	_ = xdb.View([]byte("user:1"), &u)

	fmt.Println(u.Name, u.Age)
	// Output: somak 30
}

// ExampleWithEncoder demonstrates switching to JSON encoding.
func ExampleWithEncoder() {
	xdb := badgerx.NewBadgerXDb(openDB(),
		badgerx.WithEncoder(&badgerx.JsonEncoderDecoder{}),
	)
	defer xdb.Close()

	_ = xdb.Update([]byte("user:1"), User{Name: "somak", Age: 30})

	var u User
	_ = xdb.View([]byte("user:1"), &u)

	fmt.Println(u.Name, u.Age)
	// Output: somak 30
}

// ExampleBadgerXDb_IterateView demonstrates iterating over all keys sharing
// a common prefix and collecting the decoded values into a slice.
func ExampleBadgerXDb_IterateView() {
	xdb := badgerx.NewBadgerXDb(openDB())
	defer xdb.Close()

	_ = xdb.Update([]byte("user:1"), User{Name: "alice", Age: 30})
	_ = xdb.Update([]byte("user:2"), User{Name: "bob", Age: 25})
	_ = xdb.Update([]byte("user:3"), User{Name: "carol", Age: 35})

	var users []User
	err := xdb.IterateView([]byte("user:"), badger.DefaultIteratorOptions, func(decode badgerx.DecodeFunc) error {
		var u User
		if err := decode(&u); err != nil {
			return err
		}
		users = append(users, u)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	for _, u := range users {
		fmt.Println(u.Name, u.Age)
	}
	// Output:
	// alice 30
	// bob 25
	// carol 35
}

// ExampleGobEncoderDecoder_RegisterType demonstrates registering a concrete
// type for structs that contain interface{} fields.
func ExampleGobEncoderDecoder_RegisterType() {
	type Payload struct{ Value string }
	type Record struct {
		Name    string
		Payload any
	}

	enc := &badgerx.GobEncoderDecoder{}
	enc.RegisterType(Payload{})

	xdb := badgerx.NewBadgerXDb(openDB(), badgerx.WithEncoder(enc))
	defer xdb.Close()

	_ = xdb.Update([]byte("rec:1"), Record{Name: "badgerx", Payload: Payload{Value: "hello"}})

	var r Record
	_ = xdb.View([]byte("rec:1"), &r)

	fmt.Println(r.Name, r.Payload.(Payload).Value)
	// Output: badgerx hello
}
