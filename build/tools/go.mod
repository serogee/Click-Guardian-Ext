module click-guardian/build/tools

go 1.24.1

// Keep the legacy installer tool compatible with the project toolchain.
require golang.org/x/text v0.22.0 // indirect

require (
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/mattn/go-zglob v0.0.6 // indirect
	github.com/mh-cbon/go-msi v0.0.0-20170907174732-2d172e10bc50 // indirect
	github.com/mh-cbon/stringexec v0.0.0-20160727103857-5a080a1a4118 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/urfave/cli v1.22.17 // indirect
)

tool github.com/mh-cbon/go-msi
