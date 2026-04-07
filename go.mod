module mingledb-cli

go 1.21

require github.com/mingledb/gomingleDB v0.0.0

replace github.com/mingledb/gomingleDB => ../gomingleDB

require go.mongodb.org/mongo-driver v1.17.0 // indirect
