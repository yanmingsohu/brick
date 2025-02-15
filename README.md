# Brick

A web application development framework with basic functions


## Basic use

```go
conf := brick.Config{
  HttpPort  : 7077, 
  SessionExp: 1 * time.Hour, 
  CookieName: "testserver",
  SessionDB: nil,
}
// HTTP post 7077
b := brick.NewBrick(conf)

// print call statck when error
b.Debug = true

// Redirect '/' to "/brick/ui"
b.HttpJumpMapping("/", "/brick/ui")

// static page service, auto use buildin resources
b.StaticPage("/brick/ui", "www", nil)

// http service
b.Service("/url/", func(h brick.Http) error { 
  if !h.Has("id", "text") {
    return errors.New("lost paramaters")
  }
  return nil
})

// Template with HTML
b.Service("/url/", b.TemplatePage("www/index.xhtml", 
  func(h brick.Http) (interface{}, error) { return nil, nil })

// close/shutdown when get signal
go func() {
  osSignals := make(chan os.Signal, 1)
  signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
  <-osSignals
  // b.Close()
  b.Shutdown(context.WithTimeout(context.TODO, 10 * time.Second))
}()

// start http server
b.StartHttpServer();
// https server
b.StartHttpsServer(cert, key)
```

## Template

A.xhtml file:

```html
<div>A File {{ .Data }}</div>
{{ include . "B.xhtml" }}
```

B.xhtml file:

```html
<div>B File</div>
```


## build static resource

install node version >= 6.x, run:

`npm install`

copy `build.json` file to you project, and edit.

Package static resources as go source code.

`node [brick]/build`


Static files in the compilation directory are go resource bundles
Read build.json in the current directory as the build configuration
run: execute the script without parameters nodejs > v6

The generated go code sets static resources into variables by accessing 
`b.StaticPage("/brick/ui", "www", file_mapping)`


###  Configuration instructions:

build.json file:
```
{
  "packageName": "brick",
  "fileName": "resource_www.go",
  "wwwDir": "../www",
  "outDir": "./resource",
  "varName": "file_mapping",
  "debug": false
}
```

Traverse the files in the wwwDir directory, save the file content to the varName variable,
filename is variable index; output to GO source file at outDir/fileName,
The package name is packageName; the varName variable is usually defined in other source files of the package,
variable type is map[string][]byte.
