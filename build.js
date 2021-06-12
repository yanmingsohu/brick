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
//    不支持深层目录中的文件.
//
const zb        = require('zlib')
const fs        = require('fs');
const pt        = require('path');
const cf        = require("./build.json");
const st        = require("stream");
const CleanCSS  = require('clean-css');
const htmlmin   = require('html-minifier');
const colors    = require('colors');
const babel     = require("babel-core");

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

const go_code = `
//
// 这是程序生成的资源文件
//
import (
  "io/ioutil"
  "compress/gzip"
  "bytes"
  "log"
)

func _uz(input []byte) []byte {
  r, err := gzip.NewReader(bytes.NewBuffer(input))
  if err != nil {
    log.Println("Resource fail", err)
    return nil
  }
  a, err := ioutil.ReadAll(r)
  if err != nil {
    log.Println("Resource fail", err)
    return nil
  }
  return a
}
`

var fullpath = pt.join(__dirname, cf.outDir, cf.fileName);
var outfile = makeSource(fullpath, cf.varName);
outfile.setPackage(cf.packageName);

outfile.beginInit();
buildDir([], pt.join(__dirname, cf.wwwDir), outfile, function() {
  outfile.endInit();
  console.log("\nDone", outfile.fileName);
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


function makeSource(outFile, varName) {
  var file = fs.openSync(outFile, 'w');

  return {
    fileName   : outFile,
    setPackage : setPackage,
    localfile  : localfile,
    beginInit  : beginInit,
    endInit    : endInit,
  };

  function beginInit() {
    fs.writeSync(file, go_code);
    fs.writeSync(file, "\nfunc init() {");
  }

  function endInit() {
    fs.writeSync(file, "\n}");
  }

  function setPackage(pkName) {
    fs.writeSync(file, "package ");
    fs.writeSync(file, pkName);
    fs.writeSync(file, '\n');
  }

  function localfile(path, name, over) {
    fs.writeSync(file, ['\n\n', varName, 
        '["', name, '"]=_uz([]byte{'].join(''));
    
    var wstream = fs.createWriteStream(null, {
      fd : file, 
      autoClose : false,
    });

    var r = fs.createReadStream(path);
    switch (pt.extname(path)) {
      case ".html":
        console.log(path.green);
        r = r.pipe(min_html());
        break;

      case ".js":
        console.log(path.yellow);
        r = r.pipe(min_js());
        break;

      case ".css":
        console.log(path.cyan);
        r = r.pipe(min_css());
        break;

      default:
        console.log(path.white);
        break;
    }

    r.pipe(zb.createGzip())
      .pipe(byteArrEncode(end))
      .pipe(wstream);

    wstream.on('finish', end);
      
    function end() {
      fs.writeSync(file, '})');
      over();
    }
  }
}


//
// 把二进制写出为 go 语言字节数组
//
function byteArrEncode() {
  var enc = new st.Transform();
  enc._transform = function(chunk, encoding, callback) {
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


function min_html() {
  let enc = create_collect_string(function(str, end) {
    console.time("\tHTML min");
    let result = htmlmin.minify(str, html_min_opt);
    this.push(result, 'utf8');
    console.timeLog("\tHTML min", percent(result.length, str.length));
    end();
  });
  return enc;
}


function min_js() {
  let enc = create_collect_string(function(str, end) {
    console.time("\tJS min");
    let result;
    try {
      result = babel.transform(str, js_babel_opt).code;
    } catch(err) {
      console.error(err.stack);
      result = str;
    }
    this.push(result, 'utf8');
    console.timeLog("\tJS min", percent(result.length, str.length));
    end();
  });
  return enc;
}


function min_css() {
  let enc = create_collect_string(function(str, end) {
    console.time("\tCSS min")
    var result = new CleanCSS(css_min_opt).minify(str);
    this.push(result.styles, 'utf8');
    console.timeLog("\tCSS min", percent(result.styles.length, str.length));
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
  return ((a / b)*100).toFixed(1) +'%';
}


function js_min_for_html(text, inline) {
  var start = text.match(/^\s*<!--.*/);
  var code = start ? text.slice(
    start[0].length).replace(/\n\s*-->\s*$/, '') : text;
  
  try {
    return babel.transform(code, js_babel_opt).code;
  } catch(err) {
    console.error(err.stack);
    return text;
  }
}