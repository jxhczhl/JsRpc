# JsRPC


-- js逆向之远程调用(rpc)免去抠代码补环境

> 黑脸怪
<!-- TOC -->
* [JsRPC](#jsrpc)
  * [目录结构](#目录结构)
  * [基本介绍](#基本介绍)
  * [实现](#实现)
  * [食用方法](#食用方法)
    * [打开编译好的文件，开启服务(releases下载)](#打开编译好的文件开启服务releases下载)
    * [注入JS，构建通信环境（/resouces/JsEnv_Dev.js）](#注入js构建通信环境resoucesjsenvdevjs)
    * [连接通信](#连接通信)
      * [I 远程调用0：](#i-远程调用0)
        * [接口传js代码让浏览器执行](#接口传js代码让浏览器执行)
      * [Ⅱ 远程调用1： 浏览器预先注册js方法 传递函数名调用](#ⅱ-远程调用1-浏览器预先注册js方法-传递函数名调用)
        * [远程调用1：无参获取值](#远程调用1无参获取值)
        * [远程调用2：带参获取值](#远程调用2带参获取值)
        * [远程调用3：带多个参获 并且使用post方式 取值](#远程调用3带多个参获-并且使用post方式-取值)
  * [食用案例-爬虫练手-xx网第15题](#食用案例-爬虫练手-xx网第15题)
  * [BUG修复](#bug修复)
  * [其他案例](#其他案例)
  * [常见问题](#常见问题)
  * [TODO](#todo)
<!-- TOC -->

## 目录结构

```dart
-- main.go (服务器的主代码)
-- resouces/JsEnv_Dev.js (客户端注入js环境)
```

## 基本介绍

运行服务器程序和js脚本 即可让它们通信，实现调用接口执行js获取想要的值(加解密)

## 实现

原理：在网站的控制台新建一个WebScoket客户端链接到服务器通信，调用服务器的接口 服务器会发送信息给客户端 客户端接收到要执行的方法执行完js代码后把获得想要的内容发回给服务器 服务器接收到后再显示出来

> 说明：本方法可以https证书且支持wss

在https的网站想要新建WebSocket连接如果是连接到普通的ws会报安全错误，连接本地(127.0.0.1)不会报错~ 可以用本地和wss 你自己看着玩

1. 无https证书者。直接编译main.go 使用本地ip(127.0.0.1)可以在https的网站直接连接ws使用 默认端口12080
2. 有https证书者。修改main.go文件 把r.Run()注释掉，把r.RunTls注释取消掉 并且参数设置证书的路径 直接输入名字就是当前路径 默认端口：12443


## 食用方法

### 打开编译好的文件，开启服务(releases下载)

如下图所示
![image](https://user-images.githubusercontent.com/41224971/161306799-57f009dc-5448-402f-ab4d-ee5c6c969c91.png)

1.01版本默认不打印日志
如需要打印日志，请用命令行启动（程序后面加个log） ".\rpc.exe log"

**api 简介**

- `/list` :查看当前连接的ws服务  (get)
- `/ws`  :浏览器注入ws连接的接口 (ws | wss)
- `/wst`  :ws测试使用-发啥回啥 (ws | wss)
- `/go` :获取数据的接口  (get | post)
- `/execjs` :传递jscode给浏览器执行 (get | post)

说明：接口用?group和name来区分任务 如 ws://127.0.0.1:12080/ws?group={}&name={}"
  //注入ws的例子 group和name都可以随便起名(必填)
http://127.0.0.1:12080/go?group={}&name={}&action={}&param={} //这是调用的接口
group和name填写上面注入时候的，action是注册的方法名,param是可选的参数 接口参数暂定为这几个，但是param还可以传stringify过的json(字符串) 下面会介绍

### 注入JS，构建通信环境（/resouces/JsEnv_Dev.js）

打开JsEnv 复制粘贴到网站控制台(注意：可以在浏览器开启的时候就先注入环境，不然要放开调试断点才能注入)

![image](https://user-images.githubusercontent.com/41224971/161307187-1265ec7c-fe64-45d7-b255-5472e0f25802.png)

### 连接通信

```js
// 注入环境后连接通信
var demo = new Hlclient("ws://127.0.0.1:12080/ws?group=zzz&name=hlg");
```

#### I 远程调用0：

##### 接口传js代码让浏览器执行

浏览器已经连接上通信后 调用execjs接口就行

```python
import requests

jscode = """
(function(){
    console.log("test")
    return "执行成功"
})()
"""

url = "http://localhost:12080/execjs"
data = {
    "group": "zzz",
    "name": "hlg",
    "jscode":jscode
}
res = requests.post(url, data=data)
print(res.text)
```

![image](https://user-images.githubusercontent.com/41224971/165704850-0a22dd7e-68ea-44fe-bda9-608c10795558.png)

#### Ⅱ 远程调用1： 浏览器预先注册js方法 传递函数名调用

##### 远程调用1：无参获取值

```js

// 注册一个方法 第一个参数hello为方法名，
// 第二个参数为函数，resolve里面的值是想要的值(发送到服务器的)
demo.regAction("hello", function (resolve) {
    //这样每次调用就会返回“好困啊+随机整数”
    var Js_sjz = "好困啊"+parseInt(Math.random()*1000);
    resolve(Js_sjz);
})


// 异步的代码注册：
demo.regAction('token', async (resolve) => {
  let token = await grecaptcha.execute(0, { action: '' }).then(function (token) {
    return token
  });
  resolve(token);
})

```

    访问接口，获得js端的返回值
    http://localhost:12080/go?group=zzz&name=hlg&action=hello

![image](https://user-images.githubusercontent.com/41224971/161309382-81a9a9cc-65f7-4531-a1e6-a892dfe1facd.png)

##### 远程调用2：带参获取值

```js
//写一个传入字符串，返回base64值的接口(调用内置函数btoa)
demo.regAction("hello2", function (resolve,param) {
    //这样添加了一个param参数，http接口带上它，这里就能获得
    var base666 = btoa(param)
    resolve(base666);
})
```

    访问接口，获得js端的返回值![image](https://user-images.githubusercontent.com/41224971/161311297-6731c089-3de2-44ed-80b9-21a03746a52c.png)

##### 远程调用3：带多个参获 并且使用post方式 取值

```js
//假设有一个函数 需要传递两个参数
function hlg(User,Status){
    return User+"说："+Status;
}

demo.regAction("hello3", function (resolve,param) {
    //这里还是param参数 param里面的key 是先这里写，但到时候传接口就必须对应的上
    res=hlg(param["user"],param["status"])
    resolve(res);
})
```

   访问接口，获得js端的返回值

```python
url = "http://localhost:12080/go"
data = {
    "group": "zzz",
    "name": "hlg",
    "action": "hello3",
    "param": json.dumps({"user":"黑脸怪","status":"好困啊"})
}
print(data["param"]) #dumps后就是长这样的字符串{"user": "\u9ed1\u8138\u602a", "status": "\u597d\u56f0\u554a"}
res=requests.post(url, data=data) #这里换get也是可以的
print(res.text)
```

![image](https://user-images.githubusercontent.com/41224971/161313397-166cbda0-fe8b-4063-b815-376902d82f74.png)

## 食用案例-爬虫练手-xx网第15题

    本题解是把它ajax获取数据那一个函数都复制下来，然后控制台调用这样子~

    1.f12查看请求，跟进去 找到ajax那块，可以看到call函数就是主要的ajax发包 输入页数就可以，那我们复制这个函数里面的代码备用

![image](https://user-images.githubusercontent.com/41224971/134793093-bac742e9-2f66-4fe4-b98b-7769d7379350.png)

    2.先在控制台粘贴我的js环境，再注入一个rpc链接 注册一个call方法，名字自定义 第二个参数粘贴上面call的代码，小小修改一下
       先定义num=param 这样就传参进来了，再定义一个变量来保存获取到的数据，resolve(变量) 就是发送。完了就注入好了，可以把f12关掉了

![image](https://user-images.githubusercontent.com/41224971/134795740-c62fce0d-7271-4b34-a9e5-07515b99ab81.png)

    3.调用接口就完事了，param就是传参页数

![image](https://user-images.githubusercontent.com/41224971/134799668-3dd385e7-f44c-4fb3-85ff-00d78c674865.png)

    控制台可以关，但是注入的网页不要关哦

## BUG修复

1.修复ResultSet函数，在并发处理环境下存在数据丢失，响应延迟等问题。

[//]: # (2.handlerRequest处理POST携带部分param参数调用存在JSON反序列化错误，可以使用JsEnv_Dev.js去处理)

## 其他案例

    1. JsRpc实战-猿人学-反混淆刷题平台第20题（wasm）
        https://mp.weixin.qq.com/s/DemSz2NRkYt9YL5fSUiMDQ
    2. 网洛者-反反爬练习平台第七题（JSVMPZL - 初体验）
        https://mp.weixin.qq.com/s/nvQNV33QkzFQtFscDqnXWw

## 常见问题

    1. websocket连接失败
      内容安全策略（Content Security Policy）
      Refused to connect to 'xx.xx' because it violates the following Content Security Policy directive: "connect-src 'self' 
      这个网站不让连接websocket，好像可以用油猴注入使用，或者更改网页响应头
    2. 异步操作获取值
      [参考](https://github.com/jxhczhl/JsRpc/issues/12)


## TODO

- [ ] ssl Docker Deploy
- [ ] K8s Deploy
