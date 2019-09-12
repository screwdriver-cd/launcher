module github.com/screwdriver-cd/launcher

go 1.12

require (
	github.com/mattn/go-colorable v0.1.2 // indirect
	github.com/mattn/go-isatty v0.0.9 // indirect
	github.com/myesui/uuid v1.0.0 // indirect
	github.com/peterbourgon/mergemap v0.0.0-20130613134717-e21c03b7a721
	gopkg.in/fatih/color.v1 v1.7.0
	gopkg.in/kr/pty.v1 v1.1.8
	gopkg.in/myesui/uuid.v1 v1.0.0
	gopkg.in/stretchr/testify.v1 v1.4.0 // indirect
	gopkg.in/urfave/cli.v1 v1.20.0
)

replace gopkg.in/kr/pty.v1 => github.com/kr/pty v1.1.8

replace gopkg.in/stretchr/testify.v1 => github.com/stretchr/testify v1.4.0
