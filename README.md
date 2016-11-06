# pt-rss

pt-rss是一个将PT站点rss订阅的种子下载到本地指定目录的小工具.

支持市面上大多数PT网站，例如：ttg、hdc、m-team、hdtime、tccf等.

使用go语言编写.


## 安装

1. 安装golang并设置好$GOPATH
2. 执行 go get github.com/tominescu/pt-rss/pt-rss
3. 修改 $GOPATH/src/github.com/tominescu/pt-rss/assets/sample-config.json, 填写你自己的rss地址.

## 运行

执行 $GOPATH/bin/pt-rss -c $GOPATH/github.com/tominescu/pt-rss/assets/sample-config.json

## 下载

也可下载预编译的包 https://github.com/tominescu/pt-rss/releases
