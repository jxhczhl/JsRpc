# JsRPC
##### 黑脸怪-hliang

-- js逆向之远程调用(rpc)免去抠代码补环境

> tip:懒得自己编译的 ，[releases](https://github.com/jxhczhl/JsRpc/releases)中有已经编译好的包 （win和Linux的都有~）



- [JsRPC-hliang](#jsrpc-hliang)
  - [目录结构](#目录结构)
  - [基本介绍](#基本介绍)
  - [实现](#实现)
  - [食用方法](#食用方法)
    - [打开编译好的文件，开启服务](#打开编译好的文件开启服务)
    - [注入JS，构建通信环境](#注入js构建通信环境)
    - [注入ws与方法](#注入ws与方法)
        - [远程调用1：无参获取值](#远程调用1无参获取值)
        - [远程调用2：带参获取值](#远程调用2带参获取值)
        - [远程调用3：带多个参获 并且使用post方式 取值](#远程调用3带多个参获-并且使用post方式-取值)
  - [食用案例-爱锭网15题](#食用案例-爱锭网15题)
  - [TODO](#todo)



## 目录结构
```dart
-- main.go (服务器的主代码)
-- resouces/JsEnv.js (客户端注入js环境)
```

## 基本介绍

运行服务器程序和js脚本 即可让它们通信，实现调用接口执行js获取想要的值(加解密)

## 实现

原理：在网站的控制台新建一个WebScoket客户端链接到服务器通信，调用服务器的接口 服务器会发送信息给客户端 客户端接收到要执行的方法执行完js代码后把获得想要的内容发回给服务器 服务器接收到后再显示出来

> 说明：本方法可以https证书且支持wss

在https的网站想要新建WebSocket连接如果是连接到普通的ws可能会报安全错误，好像连接本地(127.0.0.1)不会报错~ 可以用本地和wss 你自己看着玩

1. 无https证书者。直接编译main.go 我试了一下，发现使用本地ip(127.0.0.1)可以在https的网站直接连接ws使用 默认端口12080

2. 有https证书者。修改main.go文件 把r.Run()注释掉，把r.RunTls注释取消掉 并且参数设置证书的路径 直接输入名字就是当前路径 默认端口：12443

> 另外的题外话，有域名没证书不会搞的 或者有域名有公网(非固定IP的)都可以搞成的，自己研究研究

## 食用方法

### 打开编译好的文件，开启服务

如下图所示
![image](https://user-images.githubusercontent.com/41224971/161306799-57f009dc-5448-402f-ab4d-ee5c6c969c91.png)


**api 简介**

- `/list` :查看当前连接的ws服务
- `/ws`  :浏览器注入ws连接的接口
- `/go` :获取数据的接口  (可以get和post)


说明：接口用?group和name来区分任务 如 ws://127.0.0.1:12080/ws?group={}&name={}" //注入ws的例子 group和name都可以随便起名，必填选项
http://127.0.0.1:12080/go?group={}&name={}&action={}&param={} //这是调用的接口 
group和name填写上面注入时候的，action是注册的方法名,param是可选的参数 接口参数暂定为这几个，但是param还可以传stringify过的json(字符串) 下面会介绍

### 注入JS，构建通信环境

打开JsEnv 复制粘贴到网站控制台(注意有时要放开断点)

![image](https://user-images.githubusercontent.com/41224971/161307187-1265ec7c-fe64-45d7-b255-5472e0f25802.png)

### 注入ws与方法


```js
// 注入环境后连接通信
var demo = new Hlclient("ws://127.0.0.1:12080/ws?group=zzz&name=hlg");
```

##### 远程调用1：无参获取值

```js

// 注册一个方法 第一个参数hello为方法名，
// 第二个参数为函数，resolve里面的值是想要的值(发送到服务器的)
demo.regAction("hello", function (resolve) {
    //这样每次调用就会返回“好困啊+随机整数”
    var Js_sjz = "好困啊"+parseInt(Math.random()*1000);
    resolve(Js_sjz);
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
    访问接口，获得js端的返回值
![image](https://user-images.githubusercontent.com/41224971/161311297-6731c089-3de2-44ed-80b9-21a03746a52c.png)


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



## 食用案例-爱锭网15题

    本题解是把它ajax获取数据那一个函数都复制下来，然后控制台调用这样子~


    1.f12查看请求，跟进去 找到ajax那块，可以看到call函数就是主要的ajax发包 输入页数就可以，那我们复制这个函数里面的代码备用

![image](https://user-images.githubusercontent.com/41224971/134793093-bac742e9-2f66-4fe4-b98b-7769d7379350.png)

    2.先在控制台粘贴我的js环境，再注入一个rpc链接 注册一个call方法，名字自定义 第二个参数粘贴上面call的代码，小小修改一下
       先定义num=param 这样就传参进来了，再定义一个变量来保存获取到的数据，resolve(变量) 就是发送。完了就注入好了，可以把f12关掉了

![image](https://user-images.githubusercontent.com/41224971/134795740-c62fce0d-7271-4b34-a9e5-07515b99ab81.png)

    3.调用接口就完事了，param就是传参页数 

![image](https://user-images.githubusercontent.com/41224971/134799668-3dd385e7-f44c-4fb3-85ff-00d78c674865.png)

    控制台可以关，但是注入的网页不要关哦

## TODO

- [ ] ssl Docker Deploy
- [ ] K8s Deploy
