# Brick

A web application development framework with basic functions


## Basic use

```go
// HTTP post 7077
b := brick.NewBrick(port 7077, session-time)

// Redirect '/' to "/brick/ui"
b.HttpJumpMapping("/", "/brick/ui")

// static page service
b.StaticPage("/brick/ui", "www")

// start http server
b.StartHttpServer();

// http service
b.Service("/url/", func(h brick.Http) {})

// Template with HTML
b.Service("/url/", b.TemplatePage("www/index.xhtml", 
  func(h brick.Http) (interface{}, error) { return nil, nil })
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

Package static resources as go source code.

`node build`


Static files in the compilation directory are go resource bundles
Read build.json in the current directory as the build configuration
run: execute the script without parameters nodejs > v6

The generated go code sets static resources into variables by accessing `brick.file_mapping`.

###  Configuration instructions:

buiod.josn file:
```
{
  "packageName": "brick",
  "fileName": "resource_www.go",
  "wwwDir": "../www",
  "outDir": ".",
  "varName": "file_mapping"
}
```

Traverse the files in the wwwDir directory, save the file content to the varName variable,
filename is variable index; output to GO source file at outDir/fileName,
The package name is packageName; the varName variable is usually defined in other source files of the package,
variable type is map[string][]byte.
