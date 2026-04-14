module mingledb-cli

go 1.23.6

require (
	github.com/mingledb/gomingleDB v0.0.0
	github.com/reeflective/readline v1.1.4
)

replace github.com/mingledb/gomingleDB => ../gomingleDB

require (
	github.com/rivo/uniseg v0.4.4 // indirect
	go.mongodb.org/mongo-driver v1.17.0 // indirect
	golang.org/x/sys v0.8.0 // indirect
)
