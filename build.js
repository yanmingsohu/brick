//
// 编译目录中的静态文件为 go 资源包
// 读取当前目录中的 build.json 作为编译配置
// 运行: 无参数执行该脚本 nodejs > v6
//
// 配置说明: 
//    遍历 wwwDir 目录中的文件, 将文件内容保存到 varName 变量中,
//    文件名是变量索引; 输出到 outDir/fileName 的 GO 源文件中,
//    包名为 packageName; 通常在包的其他源文件中定义 varName 变量,
//    变量类型是 map[string][]byte.
//
const zb        = require('zlib')
const fs        = require('fs');
const pt        = require('path');
const st        = require("stream");
const CleanCSS  = require('clean-css');
const htmlmin   = require('html-minifier');
const babel     = require("@babel/core");
const color     = require("colors")
const cf        = require(pt.join(process.cwd(), "./build.json"));

const html_min_opt = {
  minifyCSS: true, 
  minifyJS: js_min_for_html, 
  removeComments: true,
  collapseWhitespace: true,
};

const css_min_opt = {};

const js_babel_opt = {
  minified: true, 
  comments: false,
  // plugins: ['babel-minify'],
};

const import_code = `
import (
  "io"
  "compress/gzip"
  "bytes"
  "log"
  "github.com/yanmingsohu/brick/v2"
)
`

const helper_code = `
func _unzip(input []byte) []byte {
  r, err := gzip.NewReader(bytes.NewBuffer(input))
  if err != nil {
    log.Println("Resource fail", err)
    return nil
  }
  a, err := io.ReadAll(r)
  if err != nil {
    log.Println("Resource fail", err)
    return nil
  }
  return a
}

func _unzname(i []byte) string {
	return string(_unzip(i))
}
`

var fullpath = pt.join(process.cwd(), cf.outDir, cf.fileName);
var outfile = makeSource(fullpath, cf.varName, cf.packageName, cf.debug);

outfile.fileHeader();
outfile.beginFunc("init");
buildDir([], pt.join(process.cwd(), cf.wwwDir), outfile, function() {
  outfile.endFunc();
  outfile.writeHelper();
  console.log();
  console.log("Total file size:", outfile.allSize());
  console.log("Done", outfile.fileName);
  console.log();
});


function buildDir(webbase, dir, outfile, on_end) {
  var dirs = fs.readdirSync(dir);
  var i = -1;

  _next();

  function _next() {
    if (++i < dirs.length) {
      var d = dirs[i];
      var file = pt.join(dir, d);
      var st = fs.statSync(file)

      if (st.isFile()) {
        var web_path = pt.posix.join(webbase.join('/'), d);
        outfile.localfile(file, web_path, _next);
      } 
      else if (st.isDirectory()) {
        webbase.push(d);
        buildDir(webbase, file, outfile, function() {
          webbase.pop();
          _next();
        });
      }
    } else {
      on_end();
    }
  }
}


function makeSource(outFile, varName, packageName, dbg) {
  var file = fs.openSync(outFile, 'w');
  var count = 0;
  var alltotal = 0;

  return {
    fileName   : outFile,
    setPackage : setPackage,
    localfile  : localfile,
    fileHeader : fileHeader,
    beginFunc  : beginFunc,
    endFunc    : endFunc,
    writeHelper: writeHelper,
    allSize,
  };

  function allSize() {
    return (alltotal/1024).toFixed(2) +"Kbytes"
  }

  function fileHeader() {
    fs.writeSync(file, "// generate by brick web static resource complie, ");
    fs.writeSync(file, (new Date()).toUTCString());
    fs.writeSync(file, "\n// === DO NOT === edit file.\n");
    setPackage(packageName);
    fs.writeSync(file, import_code);
    defineVar()
  }

  function writeHelper() {
    fs.writeSync(file, '\n');
    fs.writeSync(file, helper_code);
  }

  function beginFunc(name) {
    fs.writeSync(file, "\nfunc ");
    fs.writeSync(file, name);
    fs.writeSync(file, "() {");
  }

  function defineVar() {
    fs.writeSync(file, '\n// b.StaticPage("/url", "./localpath", ');
    fs.writeSync(file, varName);
    fs.writeSync(file, ')\nvar ');
    fs.writeSync(file, varName);
    fs.writeSync(file, " = brick.StaticResource{}\n")
  }

  function endFunc() {
    fs.writeSync(file, "\n}");
  }

  function setPackage(pkName) {
    fs.writeSync(file, "package ");
    fs.writeSync(file, pkName);
    fs.writeSync(file, '\n');
  }

  function localfile(path, name, over) {
    var zname = toByteArrString('[]byte', zb.gzipSync(name));

    fs.writeSync(file, "\n\n// ")
    fs.writeSync(file, name)
    fs.writeSync(file, ['\n',
      varName, '[_unzname(', zname, ')] = ([]byte{'].join(''));
    
    var wstream = fs.createWriteStream(null, {
      fd : file, 
      autoClose : false,
    });

    var stat = { gzip : 0, total: 0, min: 0, num: ++count,
                 minname: null, color:null, st:Date.now(),
                 path : path }
    
    var r = fs.createReadStream(path);
    switch (pt.extname(path)) {
      case ".html":
        stat.color = 'green'; 
        stat.minname = 'HTML';
        // console.log(path.green);
        r = r.pipe(min_html(stat));
        break;

      case ".js":
        stat.color = 'yellow';
        stat.minname = 'JS';
        // console.log(path.yellow);
        r = r.pipe(min_js(stat));
        break;

      case ".css":
        stat.color = 'cyan';
        stat.minname = 'CSS';
        // console.log(path.cyan);
        r = r.pipe(min_css(stat));
        break;

      default:
        stat.color = 'gray';
        stat.minname = "< not minify >"
        stat.total = fs.statSync(path).size;
        // console.log(path.white, color.gray("\n\t< not minify >"));
        break;
    }

    r.pipe(zb.createGzip())
      .pipe(byteArrEncode(stat))
      .pipe(wstream);

    wstream.on('finish', end);

      
    function end() {
      // console.log("\tgzip:", stat.bytes, "bytes");
      var logstr = toByteArrString('[]byte', zb.gzipSync("Web File: "+ name));
      fs.writeSync(file, '})');
      if (dbg) fs.writeSync(file, ['\nlog.Println(_unzname(', logstr, '))'].join(''))
      showStat();
      over();
      alltotal += stat.min || stat.total;
    }

    function showStat() {
      var out = [ stat.num, ') ', path,
        "\n\t", stat.minname, ' ', percent(stat.min, stat.total),
        "; gzip: ", percent(stat.gzip, stat.total), 
        "; use: ", Date.now() - stat.st, "ms"].join('');
      console.log(out[stat.color])
    }
  }
}


function toByteArrString(prefix, buf) {
  let out = [ prefix, '{' ];
  for (let i=0; i<buf.length; ++i) {
    out.push(buf[i], ',');
    if (i%20 == 0) out.push('\n');
  }
  out.push('}');
  return out.join('');
}


//
// 把二进制写出为 go 语言字节数组
//
function byteArrEncode(stat) {
  var enc = new st.Transform();
  enc._transform = function(chunk, encoding, callback) {
    stat.gzip += chunk.length;
    for (var i=0; i<chunk.length; ++i) {
      var b = chunk[i];
      this.push(b.toString());
      this.push(',');
      if (i%20 == 0) this.push('\n');
    }
    callback();
  };
  return enc;
}


function min_html(stat) {
  let enc = create_collect_string(function(str, end) {
    let result = htmlmin.minify(str, html_min_opt);
    this.push(result, 'utf8');
    stat.total = str.length;
    stat.min = result.length;
    end();
  });
  return enc;
}


function min_js(stat) {
  let enc = create_collect_string(function(str, end) {
    let result;
    try {
      result = babel.transform(str, js_babel_opt).code;
    } catch(err) {
      console.error(err.message);
      result = str;
    }
    this.push(result, 'utf8');
    stat.total = str.length;
    stat.min = result.length;
    end();
  });
  return enc;
}


function min_css(stat) {
  let enc = create_collect_string(function(str, end) {
    var result = new CleanCSS(css_min_opt).minify(str);
    this.push(result.styles, 'utf8');
    stat.total = str.length;
    stat.min = result.styles.length;
    end();
  });
  return enc;
}


function create_collect_string(cb) {
  let bufs = [];
  let enc = new st.Transform();
  enc._transform = function(chunk, encoding, callback) {
    bufs.push(chunk);
    callback();
  };

  enc._flush = function(end) {
    let str = Buffer.concat(bufs).toString('utf8')
    cb.call(this, str, end);
  };
  return enc;
}


function percent(a, b) {
  if (a <= 0) return "";
  return ((a / b)*100).toFixed(1) +'%, '+ (a/1024).toFixed(2) +"Kbytes";
}


function js_min_for_html(text, inline) {
  var start = text.match(/^\s*<!--.*/);
  var code = start ? text.slice(
    start[0].length).replace(/\n\s*-->\s*$/, '') : text;
  
  try {
    return babel.transform(code, js_babel_opt).code;
  } catch(err) {
    console.error(err.message);
    return text;
  }
}