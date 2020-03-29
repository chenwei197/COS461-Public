/*****************************************************************************
 * http_proxy_DNS.go                                                                 
 * Names: 
 * NetIds:
 *****************************************************************************/

// TODO: implement an HTTP proxy with DNS Prefetching

// Note: it is highly recommended to complete http_proxy.go first, then copy it
// with the name http_proxy_DNS.go, thus overwriting this file, then edit it
// to add DNS prefetching (don't forget to change the filename in the header
// to http_proxy_DNS.go in the copy of http_proxy.go)
package main

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"golang.org/x/net/html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func main() {
	var addr string
	fmt.Scan(&addr) //输入端口号
	addr = ":" + addr
	listener, err := net.Listen("tcp", addr) //开始监听该端口号
	CheckError(err, "net listen")
	for {
		conn, err := listener.Accept() //建立连接
		if err != nil {
			continue
		}
		go HandleClient(conn) //启动子线程处理从客户端发来的http请求报文，从而实现多用户同时访问的目的
	}
}

//处理从客户端发来的http请求报文
func HandleClient(conn net.Conn) {
	data := make([]byte, 1024)
	len, err := conn.Read(data)
	CheckError(err, "read client data")
	str := string(data[:len])
	stringReader := strings.NewReader(str)
	reader := bufio.NewReader(stringReader)
	request, err := http.ReadRequest(reader) //http数据封装成request
	CheckError(err, "data to request")
	if request.Method == "GET" { //只代理GET方法，其他方法都返回状态500
		HandleServer(request, conn)
	} else {
		data := "HTTP/1.1 500\r\n"
		data += "Connection: close\r\n"
		data += "\r\n\r\n"
		conn.Write([]byte(data))
		conn.Close()
	}
}

//负责和服务器交互
func HandleServer(request *http.Request, connClient net.Conn) {
	defer connClient.Close()

	data := request.Method + " " + request.URL.Path + " HTTP/1.1\r\n" //根据request创建一个http请求数据包，并设connection字段为close
	data += "HOST: " + request.Host + "\r\n"
	request.Header.Set("Connection", "close")
	for k, v := range request.Header {
		data += k + ": " + v[0] + "\r\n"
	}
	body := make([]byte, 1024)
	request.Body.Read(body)
	data += "\r\n" + string(body) + "\r\n"

	if strings.Index(request.Host, ":") == -1 { //如果没有声明端口号，加上默认端口号80
		request.Host = request.Host + ":80"
	}
	conn, err := net.Dial("tcp", request.Host) //与服务器建立tcp连接
	CheckError(err, "connect to server")
	defer conn.Close()

	conn.Write([]byte(data)) //向服务器发送前面创建的http请求数据包
	buf := make([]byte, 1024)
	data = ""
	for { //接收服务器发送的http响应数据包
		n, err := conn.Read(buf)
		data += string(buf[:n])
		if err != nil {
			break
		}
	}
	//go PrefetchDNS(data)	//启动子进程解析html部分的url，并预取DNS，不影响返回给客户端的响应
	connClient.Write([]byte(data)) //向客户端发送http响应数据包
}

//部分http响应数据包的主体部分是通过gzip压缩的数据，要先解压
func HandleGzip(body io.ReadCloser) (bodyReader io.Reader) {
	gzipReader, err := gzip.NewReader(body)
	CheckError(err, "create gzipReader")
	defer gzipReader.Close()
	buf := make([]byte, 1024)
	data := ""
	for {
		n, err := gzipReader.Read(buf)
		data += string(buf[:n])
		if err != nil {
			break
		}
	}
	return strings.NewReader(data)
}

//处理http响应报文中的主体部分，如果是html数据，找到其中的嵌入的域名，并解析
func PrefetchDNS(data string) {
	stringReader := strings.NewReader(data)
	bufReader := bufio.NewReader(stringReader)
	response, err := http.ReadResponse(bufReader, nil)
	CheckError(err, "read response")
	defer response.Body.Close()
	var bodyReader io.Reader
	if response.Header.Get("Content-Type") != "text/html" { //如果http响应报文的主体不是html数据，则不用预取DNS
		return
	}
	if response.Header.Get("Content-Encoding") == "gzip" { //如果http响应报文的主体编码为gzip，则需要先解压
		bodyReader = HandleGzip(response.Body)
	} else {
		bodyReader = response.Body
		CheckError(err, "read response body")
	}
	doc, err := html.Parse(bodyReader)
	CheckError(err, "html to document")
	FinddHref(doc) //寻找html中的a标签，并解析其中的域名
}

//寻找html中的标签，输入为html document的树结构
func FinddHref(n *html.Node) {
	if n.Type == html.ElementNode && n.Data == "a" {
		for _, a := range n.Attr {
			if a.Key == "href" {
				u, err := url.Parse(a.Val)
				CheckError(err, "parse url")
				fmt.Println("DNS预取：", u.Host)
				net.LookupHost(u.Host)
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling { //迭代
		FinddHref(c)
	}
}

//处理错误
func CheckError(err error, pst string) {
	if err != nil {
		fmt.Fprint(os.Stderr, "Fatal error in ", pst, " : ", err.Error())
		fmt.Println()
		os.Exit(1)
	}
}
