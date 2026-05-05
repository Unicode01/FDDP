module fddp-demo/backend

go 1.22

require (
	github.com/Unicode01/FDDP/packages/go-fddp v0.0.0
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.31.1
)

require (
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/mattn/go-sqlite3 v1.14.22 // indirect
	golang.org/x/text v0.20.0 // indirect
)

replace github.com/Unicode01/FDDP/packages/go-fddp => ../../../packages/go-fddp
