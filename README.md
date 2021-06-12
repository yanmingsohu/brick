# Brick

A web application development framework with basic functions


## Basic use

```go
// HTTP post 7077
b := brick.NewBrick(7077)

// Redirect '/' to "/brick/ui"
b.HttpJumpMapping("/", "/brick/ui")

// static page service
b.StaticPage("/brick/ui", "www", "index.html")

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