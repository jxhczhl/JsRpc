function Hlclient(wsURL) {
    this.wsURL = wsURL;
    this.handlers = {};
    this.socket = {};

    if (!wsURL) {
        throw new Error('wsURL can not be empty!!')
    }
    this.connect()
}

Hlclient.prototype.connect = function () {
    console.log('begin of connect to wsURL: ' + this.wsURL);
    var _this = this;
    try {
        this.socket["ySocket"] = new WebSocket(this.wsURL);
        this.socket["ySocket"].onmessage = function (e) {
            console.log("send func", e.data);
            _this.handlerRequest(e.data);
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

}
Hlclient.prototype.handlerRequest = function (requestJson) {
	var _this = this;
	var result=JSON.parse(requestJson);
	//console.log(result)
	if (!result['action']) {
        this.sendResult('','need request param {action}');
        return
    }
	action=result["action"]
    var theHandler = this.handlers[action];
    try {
		if (!result["param"]){
			theHandler(function (response) {
				_this.sendResult(action, response);
			})
		}else{
			theHandler(function (response) {
				_this.sendResult(action, response);
			},result["param"])
		}
		
    } catch (e) {
        console.log("error: " + e);
        _this.sendResult(action+e);
    }
}

Hlclient.prototype.sendResult = function (action, e) {
    this.send(action + atob("aGxeX14") + e);
}
