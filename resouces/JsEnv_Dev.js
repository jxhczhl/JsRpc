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
    this.socket = {};
    if (!wsURL) {
        throw new Error('wsURL can not be empty!!')
    }
    this.connect()
    this.socket["ySocket"].addEventListener('close', (event) => {
        console.log('rpc已关闭');
    });
}

Hlclient.prototype.connect = function () {
    console.log('begin of connect to wsURL: ' + this.wsURL);
    var _this = this;
    try {
        this.socket["ySocket"] = new WebSocket(this.wsURL);
        this.socket["ySocket"].onmessage = function (e) {
            try {
                let blob = e.data
                blob.text().then(data => {
                    _this.handlerRequest(data);
                })
            } catch {
                console.log("not blob")
                _this.handlerRequest(blob)
            }

        }
    } catch (e) {
        console.log("connection failed,reconnect after 10s");
        setTimeout(function () {
            _this.connect()
        }, 10000)
    }
    this.socket["ySocket"].onclose = function () {
        console.log("connection failed,reconnect after 10s");
        setTimeout(function () {
            _this.connect()
        }, 10000)
    }
    this.socket["ySocket"].addEventListener('open', (event) => {
        console.log("rpc连接成功");
    });
    this.socket["ySocket"].addEventListener('error', (event) => {
        console.error('rpc连接出错,请检查是否打开服务端:', event.error);
    });

};
Hlclient.prototype.send = function (msg) {
    this.socket["ySocket"].send(msg)
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
        } else {
            var param = result["param"]
            try {
                param = JSON.parse(param)
            } catch (e) {
                console.log("")
            }
            theHandler(function (response) {
                _this.sendResult(action, response);
            }, param)
        }

    } catch (e) {
        console.log("error: " + e);
        _this.sendResult(action + e);
    }
}

Hlclient.prototype.sendResult = function (action, e) {
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