package brick

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/kataras/go-sessions"
)

var (
	ErrLogNullPoint = errors.New("null log point")
	ErrBindOutParam = errors.New("not out bind param")
	ErrBindNotFix = errors.New("not found 'fixBase' in URL")
)

type Msg struct {
  Code int          `json:"code"`
  Msg  string       `json:"msg"`
  Data interface{}  `json:"data"`
}

type HttpError struct {
	Code int
	error
}

type Shutdown interface {
  Close()
}

type Logger interface {
  Println(v ...any)
	Printf(format string, v ...any)
	Panicln(v ...any)
}

//
// 方便编写 http 服务
//
type Brick struct {
  sess            *sessions.Sessions
  secureCookie    *securecookie.SecureCookie
  addr            string
  serveMux        *http.ServeMux
  funcMap         template.FuncMap
  cachedTemplate  map[string]*CachedTemplate
  tplLock         sync.Mutex
  templateDir     string
  log             Logger
  errorHandle     HttpErrorHandler
  Debug           bool
	serv            http.Server
	staticCacheSec  int
} 

type Http struct {
  R  *http.Request
  W  http.ResponseWriter
  b  *Brick
  s  *sessions.Session
  c  []Shutdown
  q  *url.Values
  // 在记录 http 日志时的附加条目
  L  string
}

type StaticResource map[string][]byte

type StaticPage struct {
  BaseUrl    string // web 服务的路径前缀
  FilePath   string // 本地文件路径
  localFS    http.Handler
  log        Logger
	mapping    StaticResource
	debug      *bool
	cacheSec   int
}

//
// 已经缓存的模板对象
//
type CachedTemplate struct {
  lastTime time.Time
  fileName string
  template *template.Template
}

//
// HTML 模板上下文, 即模板中 '.' 符号表示的实例, 
// '.Data' 是 TemplateHandler 函数返回的数据.
//
type TplFuncCtx struct {
  io.Writer
  Data    *interface{}
  Dirname string
  parent  *template.Template
}

//
// html 模板处理函数, 该函数准备渲染模板需要的数据, 并在第一个参数返回
// 如果出错, 返回第二个参数, 此时错误会输出到客户端, 并终止模板渲染
// HEAD 请求不会渲染模板.
//
type TemplateHandler func(*Http)(interface{}, error)

//
// http 服务处理函数, 在可能返回 error 之前不要写出任何数据
// 返回的 error 会设置输出为 500 http code
//
type HttpHandler func(*Http) error

//
// 当发生 http 异常或 HttpHandler 返回错误, 对错误执行这个方法
// 通常记录日志并向客户端输出错误信息
//
type HttpErrorHandler func(hd *Http, err interface{})


type Database = sessions.Database
type LifeTime = sessions.LifeTime


type Config struct {
	HttpPort int
	SessionExp time.Duration
	CookieName string
	SessionDB Database
	Log Logger
	SessionHashKey []byte
	SessionBlockKey []byte
	// 如果缓存时间 == 0, 则文件一直被缓存
	StaticCacheSeconds int
	ErrorHandle HttpErrorHandler
}


func (c *Config) DefaultValue() {
	if c.SessionHashKey == nil {
		c.SessionHashKey = securecookie.GenerateRandomKey(32)
	}
	if c.SessionBlockKey == nil {
		c.SessionBlockKey = securecookie.GenerateRandomKey(16)
	}
	if c.Log == nil {
		c.Log = log.New(os.Stdout, "HT.", log.LstdFlags | log.Lmsgprefix)
	}
	if c.SessionExp <= 0 {
		c.SessionExp = 2 * time.Hour
	}
}

//
// 创建 Brick 的实例, session 对象在 sessionExp 后无效.
//
func NewBrick(conf Config) *Brick {
	conf.DefaultValue()

	mux := http.NewServeMux()
	hport := ":"+ strconv.Itoa(conf.HttpPort);
	hname, err := os.Hostname()
	if err != nil {
		hname = "localhost:"+ hport
	} else {
		hname += hport
	}

	secureCookie := securecookie.New(conf.SessionHashKey, conf.SessionBlockKey)
	eh := conf.ErrorHandle
	if eh == nil {
		eh = defaultErrorHandle
	}

  b := Brick{ 
    addr       			: hname,
    secureCookie    : secureCookie,
    cachedTemplate  : make(map[string]*CachedTemplate),
    serveMux        : mux,
    funcMap         : template.FuncMap{},
    log             : conf.Log,
    errorHandle     : eh,
		serv 						: http.Server{Addr: hport, Handler: mux},
		staticCacheSec  : conf.StaticCacheSeconds,
  
    sess: sessions.New(sessions.Config{
      Cookie: conf.CookieName,
      Expires: conf.SessionExp,
      Encode: secureCookie.Encode,
      Decode: secureCookie.Decode,
    }),
  }

	if conf.SessionDB != nil {
		b.sess.UseDatabase(conf.SessionDB)
	}

  b.defaultTemplateFunc()
  return &b;
}


func (b *Brick) defaultTemplateFunc() {
  b.funcMap["include"] = func(fc TplFuncCtx, filename string)(string, error) {
    fn := filepath.Join(fc.Dirname, filename)
    ct, err := b.GetCachedTemplate(fn)
    if err != nil {
      return "", err
    }
    nfc := TplFuncCtx{ fc, fc.Data, filepath.Dir(fn), ct.template }
    if err := ct.template.Execute(nfc, nfc); err != nil {
      return "", err
    }
    return "", nil
  }
}


func (b *Brick) SetTplFunc(name string, fn interface{})(error) {
  if fn == nil {
    b.log.Println("ERR. Template Function not nil")
  }
  b.funcMap[name] = fn
  return nil
}


//
// 启动服务, 该方法会阻塞
//
func (b *Brick) StartHttpServer() error {
  b.log.Println("Server on http://"+ b.addr)
	return b.serv.ListenAndServe()
}


func (b *Brick) StartHttpsServer(cert string, key string) error {
  b.log.Println("Server on https://"+ b.addr)
	return b.serv.ListenAndServeTLS(cert, key)
}


func (b *Brick) Close() error {
	return b.serv.Close()
}


func (b *Brick) Shutdown(ctx context.Context) error {
	return b.serv.Shutdown(ctx)
}


//
// 启用并返回事务对象
//
func (h *Http) Session()(*sessions.Session) {
  if h.s == nil {
    h.s = h.b.sess.Start(h.W, h.R)
  } 
  return h.s
}


//
// 普通 web 服务
//
func (b *Brick) Service(path string, h HttpHandler) {
  if b.Debug {
		b.log.Println("Add Service", path)
	}
	
  b.serveMux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
    t1 := time.Now()
    hd := Http{ r, w, b, nil, make([]Shutdown, 0, 3), nil, "" }

    defer func() {
      if err := recover(); err != nil {
        if b.Debug {
          var buf [4096]byte
          n := runtime.Stack(buf[:], false)
          b.log.Println("==>", err, string(buf[:n]))
        }

        b.errorHandle(&hd, err)
      }
    }()
    
		w.Header().Add("Cache-Control", "no-store")
    if err := h(&hd); err != nil {
      b.errorHandle(&hd, err)
    }
    hd.shutdown()

		if b.Debug {
    	serviceLog(b.log, t1, r, hd.L)
		}
  })
}


func defaultErrorHandle(hd *Http, err interface{}) {
  hd.W.WriteHeader(500)
  hd.WriteStr(`<p>Service Error</p>`)
  fmt.Fprintf(hd.W, `<p>%s</p>`, err)
  hd.b.log.Println("Error:", err)
}


//
// 设置 html 模板文件加载目录
//
func (b *Brick) SetTemplateDir(path string) {
  b.templateDir = path
}


//
// 编译并返回 html 模板对象, 如果模板文件有变更, 会重新编译
// TODO: 支持模板之间的 define/template 调用
//
func (b *Brick) GetCachedTemplate(fileName string)(*CachedTemplate, error) {
  modtime, file, err := lastModifyTime(fileName);
  if err != nil {
    return nil, err
  }
  defer file.Close() 

  b.tplLock.Lock()
  defer b.tplLock.Unlock()
  cd := b.cachedTemplate[fileName]
  if cd == nil {
    cd = &CachedTemplate{}
    b.cachedTemplate[fileName] = cd
  }

  if !modtime.Equal(cd.lastTime) {
    b.log.Println("Template change", fileName)
    buf, errR := io.ReadAll(file)
    if errR != nil {
      return nil, errR
    }

    cd.template = template.New(fileName).Funcs(b.funcMap)
    if _, errP := cd.template.Parse(string(buf)); errP != nil {
      return nil, errP
    }
    cd.lastTime = *modtime
    cd.fileName = fileName
  }
  return cd, nil
}


//
// 创建模板服务 handle 返回的上下文对象中的数据绑定到 
// template_file 指定的模板中, 服务映射到 url 路径上.
// 如果使 HTTP HEAD 请求, 模板不会渲染, 如果没有错误则返回 204
//
func (b *Brick) TemplatePage(
    templateFile string, handle TemplateHandler)(HttpHandler) {
	if b.Debug {
   	b.log.Println("Template", templateFile)
	}
  dir := filepath.Dir(templateFile)

  return func(hd *Http) error {
		hd.W.Header().Add("Cache-Control", "private")
		hd.W.Header().Add("Cache-Control", "max-age="+ strconv.Itoa(b.staticCacheSec))
    hd.W.Header().Set("Content-Type", "text/html; charset=utf-8")
    ct, err := b.GetCachedTemplate(templateFile)
    if err != nil {
      hd.WriteStr("Parse Template Error<br/>")
      return err
    }

    data, errTC := handle(hd)
    if errTC != nil {
      return errTC
    }
    if hd.R.Method == "HEAD" {
      hd.W.WriteHeader(204)
      return nil
    }

    fc := TplFuncCtx{ hd.W, &data, dir, ct.template }
    if err := ct.template.Execute(hd.W, fc); err != nil {
      return err
    }
    return nil
  }
}


//
// 把对 location 的请求跳转到 to 上, 
// 如果参数 location == '/', 则对没有注册过的路径的请求都会转发到 to 上.
//
func (b *Brick) HttpJumpMapping(location string, to string) {
  b.serveMux.HandleFunc(location, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", "no-store")
    if r.Method == "HEAD" {
      w.WriteHeader(405)
      return
    }
    http.Redirect(w, r, to, http.StatusMovedPermanently)
  })
}


//
// 设置静态文件服务, 必须在该方法之前设置 log 否则无效
// eh 可用为 nil, 否则在遇到错误时会回掉该方法
//
func (b *Brick) StaticPage(baseURL string, fileDir string, res StaticResource) {
  if (!strings.HasSuffix(baseURL, "/")) {
    baseURL = baseURL + "/"
  }
  local := http.StripPrefix(baseURL, http.FileServer(http.Dir(fileDir)));

	if b.errorHandle != nil {
		local = &WrapErrorHandler{ local, b.errorHandle, b, nil }
	}

  staticPage := StaticPage {
		BaseUrl		: baseURL,
		FilePath	: fileDir,
    localFS   : local,
    log       : b.log,
		mapping   : res,
		debug     : &b.Debug,
		cacheSec  : b.staticCacheSec,
  };
  b.serveMux.Handle(baseURL, &staticPage);
}


type WrapErrorHandler struct {
	src http.Handler
	eh  HttpErrorHandler
	b   *Brick
	http.ResponseWriter
}


func (w *WrapErrorHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	_resp := &WrapErrorResponse{ resp, 0, make([]byte, 0, 20) }
	w.src.ServeHTTP(_resp, req)

	if _resp.haserr != 0 {
		err := HttpError{ _resp.haserr, errors.New(string(_resp.errmsg)) }
		hd := Http{ req, resp, w.b, nil, nil, nil, "" }
		w.eh(&hd, err)
	}
}


// 如果设置了 http 错误代码, 会拦截输出作为错误源
type WrapErrorResponse struct {
	http.ResponseWriter
	// 0 表示没有错误
	haserr int
	errmsg []byte
}


func (w *WrapErrorResponse) WriteHeader(statusCode int) {
	if statusCode >= 400 {
		w.haserr = statusCode
	} else {
		w.ResponseWriter.WriteHeader(statusCode)
	}
}


func (w *WrapErrorResponse) Write(b []byte) (int, error) {
	if w.haserr != 0 {
		w.errmsg = append(w.errmsg, b...)
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}


//
// 设置 brick 打印日志的目标对象
//
func (b *Brick) SetLogger(log Logger) {
  if log == nil {
    panic(ErrLogNullPoint)
  }
  b.log = log
}


//
// 返回 json 字符串
//
func (h *Http) Json(m interface{}) {
  h.W.Header().Set("Content-Type", "application/json; charset=utf-8")
  enc := json.NewEncoder(h.W)
	if err := enc.Encode(m); err != nil {
		h.W.WriteHeader(500)
		h.W.Write([]byte("server error 500"))
		if h.b.Debug {
			h.W.Write([]byte(err.Error()))
		}
		h.b.log.Println("http write json fail:", err)
	}
}


func (h* Http) init_query() {
  if h.q == nil {
    ct := h.R.Header.Get("Content-Type")
    if strings.Contains(ct, "application/x-www-form-urlencoded") {
      h.R.ParseForm()
      h.q = &h.R.PostForm
    } else {
      q := h.R.URL.Query()
      h.q = &q
    }
  }
}


//
// 返回 URI 中的参数, 参数为空返回空字符串
//
func (h* Http) Get(name string) string {
  h.init_query()
  return h.q.Get(name)
}


func (h* Http) Gets(name string) []string {
  h.init_query()
  return (*h.q)[name]
}


func (h *Http) Has(name ...string) bool {
	h.init_query()
	for _,n := range name {
		if !h.q.Has(n) {
			return false
		}
	}
	return true
}


func (h *Http) GetF(name string) float64 {
	h.init_query()
	s := h.q.Get(name)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		h.b.log.Panicln("bad paramater:", name, "not float:", s)
	}
	return f
}


func (h *Http) GetI(name string) int64 {
	h.init_query()
	s := h.q.Get(name)
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		h.b.log.Panicln("bad paramater:", name, "not integer:", s)
	}
	return i
}


func (h *Http) GetB(name string) bool {
	h.init_query()
	s := h.q.Get(name)
	b, _ := strconv.ParseBool(s)
	return b
}


//
// 输出错误字符串, 该方法不影响程序流程
//
func (h *Http) WriteErr(e error) {
  h.b.log.Println("ERR.", e)
  h.W.Write([]byte(e.Error()))
}


func (h *Http) WriteStr(s string) {
  h.W.Write([]byte(s))
}


//
// 写一个 css 引用标签
//
func (h *Http) WriteCSS(href string) {
  fmt.Fprintf(h.W, 
    "<link type='text/css' href='%s' rel='stylesheet'/>", href);
}


//
// 解析 url, 从 fixBase 片段开始并把 url 中的路径片段绑定到 out 的输出参数
// ULR 绑定规则示意: "/someother../fixBase/out1/out2/*"
// 如果输入 url 片段数量多于 out 的数量, 
// 后面的 url 片段被丢弃, 返回丢弃的数量
// 如果输入 url 片段数量少于 out 的数量, 
// 后面的 out 不改变原始值, 返回多余的 out 数量的负值
// 返回值 == 0 说明 url 参数和 out 数量匹配
// 如果在 url 中找不到 fixBase 路径片段则发生异常
//
func (h *Http) URLParam(fixBase string, out ...*string) int {
  arr := strings.Split(h.R.URL.Path, "/")
  ulen := len(arr)
  olen := len(out)
  ui := 0

  if olen <= 0 {
    panic(ErrBindOutParam)
  }

  for ; ui < ulen; ui++ {
    if arr[ui] == fixBase {
      ui++
      break
    }
  }

  ucount := ulen - ui
  if ucount == 0 {
    panic(ErrBindNotFix)
  }

  oi := 0
  for ; oi < olen && ui < ulen; oi++ {
    *(out[oi]) = arr[ui]
    ui++
  }
  return ucount - olen
}


//
// 当 http 响应结束, 所有注册的 Shutdown 接口都被调用
//
func (h *Http) CloseOnEnd(c Shutdown) {
  h.c = append(h.c, c)
}


func (h *Http) shutdown() {
  for _, c := range h.c {
    c.Close()
  }
}


//
// 输出纯文本 HTML 标签
//
func (h *Http) TextTag(tagName string, text string, attr ...string) {
  h.Tag(tagName, func() {
    h.WriteStr(text)
  }, attr...)
}


//
// 输出 HTML 标签, 属性长度必须是偶数
//
func (h *Http) Tag(tagName string, body func(), attr ...string) {
  h.WriteStr("<")
  h.WriteStr(tagName)
  for i:=0; i<len(attr); i+=2 {
    h.WriteStr(" ")
    h.WriteStr(attr[i])
    h.WriteStr("=\"")
    h.WriteStr(attr[i+1])
    h.WriteStr("\"")
  }
  h.WriteStr(">")
  body()
  h.WriteStr("</")
  h.WriteStr(tagName)
  h.WriteStr(">")
}


//
// 只返回首选 AcceptLanguage
//
func (h *Http) GetAcceptLanguage()(string) {
  al := h.R.Header.Get("Accept-Language")
  ar := strings.Split(al, ",")
  if len(ar) < 1 {
    return ""
  }
  return ar[0]
}


//
// 设置 http 头域中的缓存时间字段, 应该在写出任何内容之前设置
//
func (h *Http) CacheTime(d time.Duration) {
  var cc string
  if d <= 0 {
    cc = "no-store"
  } else {
    cc = "max-age="+ strconv.FormatFloat(d.Seconds(), 'f', 0, 64)
  }
  h.W.Header().Set("Cache-Control", cc)
}


//
// Brick.GetCachedTemplate() 的简写
//
func (h *Http) GetTpl(filename string)(*template.Template, error) {
  tpl, err := h.b.GetCachedTemplate(filepath.Join(h.b.templateDir, filename))
  if err != nil {
    return nil, err
  }
  t := tpl.lastTime.Format(time.RFC1123Z)
  h.W.Header().Set("Last-Modified", t +" GMT")
  return tpl.template, nil
}


//
// 返回 http 请求上下文
//
func (h *Http) Ctx() context.Context {
  return h.R.Context()
}


func (b *Http) SetDownloadFilename(s string) {
	filename := url.QueryEscape(s)
	b.W.Header().Add("Content-Disposition", 
		"attachment; filename=\""+ filename +"\";"+
		"filename*=utf-8''"+ filename)
}


func (p *StaticPage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
  fileName := r.URL.Path[len(p.BaseUrl):]
  begin    := time.Now()

	if p.mapping != nil {
  	content, has := p.mapping[fileName]
		if has {
			// log.Println("Prog Resource", fileName)
			w.Header().Add("Cache-Control", "public, max-age="+ strconv.Itoa(p.cacheSec))
			w.Header().Set("Content-Type", getMimeType(fileName))
			w.Header().Set("Content-Encoding", "gzip")
			w.WriteHeader(200)
			w.Write(content)
			
			if *p.debug {
				serviceLog(p.log, begin, r, "[mapping]");
			}
			return;
		}
	}

	w.Header().Add("Cache-Control", "no-cache")
	p.localFS.ServeHTTP(w, r)
  if *p.debug { 
		serviceLog(p.log, begin, r, "[fs]"); 
	}
}


func lastModifyTime(filename string) (*time.Time, *os.File, error) {
  file, err := os.Open(filename);
  if err != nil {
    return nil, nil, err
  }

  stat, errS := file.Stat()
  if errS != nil {
    return nil, nil, errS
  }
  t := stat.ModTime()
  return &t, file, nil
}


func getMimeType(fileName string) string {
  ctype := mime.TypeByExtension(filepath.Ext(fileName))
  if ctype == "" {
    ctype = "application/octet-stream"
  }
  return ctype
}


//
// 取 str 字符串末尾 maxLen 指定的长度, 
// 如果 str 长度小于 maxLen 则返回 str 切末尾补充空格
// 如果发生截断, 则前面加 prefix 符号
//
func LastSlice(str string, maxLen int, prefix string) string {
  l := len(str)
  if l > maxLen {
    return prefix + str[l - maxLen + len(prefix):]
  } else if l < maxLen {
    n := make([]byte, maxLen)
    copy(n[0:], str)
    return string(n)
  }
  return str
}


func serviceLog(log Logger, begin time.Time, r *http.Request, extLog string) {
  log.Printf("%4s|%12s|%s %s", 
        LastSlice(r.Method, 4, ""), 
        time.Since(begin).String(), 
        r.URL.Path,
        extLog)
}
