function Hlclient(wsURL) {
    this.wsURL = wsURL;
    this.handlers = {
        _execjs: function (resolve, param) {
            var res = eval(param)
            if (!res) {
                resolve("没有返回值")
            } else {
                resolve(res)
            }

        }
    };
    this.socket = undefined;
    if (!wsURL) {
        throw new Error('wsURL can not be empty!!')
    }
    this.connect()
}

Hlclient.prototype.connect = function () {
    console.log('begin of connect to wsURL: ' + this.wsURL);
    var _this = this;
    try {
        this.socket = new WebSocket(this.wsURL);
        this.socket.onmessage = function (e) {
            _this.handlerRequest(e.data)
        }
    } catch (e) {
        console.log("connection failed,reconnect after 10s");
        setTimeout(function () {
            _this.connect()
        }, 10000)
    }
    this.socket.onclose = function () {
        console.log('rpc已关闭');
        setTimeout(function () {
            _this.connect()
        }, 10000)
    }
    this.socket.addEventListener('open', (event) => {
        console.log("rpc连接成功");
    });
    this.socket.addEventListener('error', (event) => {
        console.error('rpc连接出错,请检查是否打开服务端:', event.error);
    });

};
Hlclient.prototype.send = function (msg) {
    this.socket.send(msg)
}

Hlclient.prototype.regAction = function (func_name, func) {
    if (typeof func_name !== 'string') {
        throw new Error("an func_name must be string");
    }
    if (typeof func !== 'function') {
        throw new Error("must be function");
    }
    console.log("register func_name: " + func_name);
    this.handlers[func_name] = func;
    return true

}

//收到消息后这里处理，
Hlclient.prototype.handlerRequest = function (requestJson) {
    var _this = this;
    try {
        var result = JSON.parse(requestJson)
    } catch (error) {
        console.log("catch error", requestJson);
        result = transjson(requestJson)
    }
    //console.log(result)
    if (!result['action']) {
        this.sendResult('', 'need request param {action}');
        return
    }
    var action = result["action"]
    var theHandler = this.handlers[action];
    if (!theHandler) {
        this.sendResult(action, 'action not found');
        return
    }
    try {
        if (!result["param"]) {
            theHandler(function (response) {
                _this.sendResult(action, response);
            })
            return
        }
        var param = result["param"]
        try {
            param = JSON.parse(param)
        } catch (e) {
            console.log("")//不是json无需操作
        }
        theHandler(function (response) {
            _this.sendResult(action, response);
        }, param)

    } catch (e) {
        console.log("error: " + e);
        _this.sendResult(action, e);
    }
}

Hlclient.prototype.sendResult = function (action, e) {
    if (typeof e === 'object' && e !== null) {
        try {
            e = JSON.stringify(e)
        } catch (v) {
            console.log(v)//不是json无需操作
        }
    }
    this.send(action + atob("aGxeX14") + e);
}

function transjson(formdata) {
    var regex = /"action":(?<actionName>.*?),/g
    var actionName = regex.exec(formdata).groups.actionName
    stringfystring = formdata.match(/{..data..:.*..\w+..:\s...*?..}/g).pop()
    stringfystring = stringfystring.replace(/\\"/g, '"')
    paramstring = JSON.parse(stringfystring)
    tens = `{"action":` + actionName + `,"param":{}}`
    tjson = JSON.parse(tens)
    tjson.param = paramstring
    return tjson
}
