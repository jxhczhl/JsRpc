var rpc_client_id, Hlclient = function (wsURL) {
    this.wsURL = wsURL;
    this.handlers = {
        _execjs: function (resolve, param) {
            var res = eval(param)
            resolve(res || "没有返回值")
        }
    };
    this.socket = undefined;
    this.isWechat = typeof wx !== 'undefined'; // 新增环境判断

    if (!wsURL) throw new Error('wsURL can not be empty!!');

    // 微信环境读取持久化的clientId
    if (this.isWechat && wx.getStorageSync('rpc_client_id')) {
        rpc_client_id = wx.getStorageSync('rpc_client_id');
    }

    this.connect();
}

Hlclient.prototype.connect = function () {
    var _this = this;

    // 处理clientId参数
    if (this.wsURL.indexOf("clientId=") === -1 && rpc_client_id) {
        this.wsURL += "&clientId=" + rpc_client_id;
    }

    console.log('begin connect to:', this.wsURL);

    try {
        if (this.isWechat) {
            // 微信环境使用wx API
            this.socket = wx.connectSocket({
                url: this.wsURL,
                success() {
                    console.log('微信WS连接建立成功');
                },
                fail(err) {
                    console.error('微信WS连接失败:', err);
                    _this.reconnect();
                }
            });

            // 微信事件监听
            wx.onSocketMessage(function (res) {
                _this.handlerRequest(res.data);
            });

            wx.onSocketOpen(function () {
                console.log("rpc连接成功");
            });

            wx.onSocketError(function (err) {
                console.error('rpc连接出错:', err);
            });

            wx.onSocketClose(function () {
                console.log('rpc连接关闭');
                _this.reconnect();
            });

        } else {
            // 浏览器环境
            this.socket = new WebSocket(this.wsURL);

            this.socket.onmessage = function (e) {
                _this.handlerRequest(e.data);
            }

            this.socket.onclose = function () {
                console.log('rpc已关闭');
                _this.reconnect();
            }

            this.socket.addEventListener('open', () => {
                console.log("rpc连接成功");
            });

            this.socket.addEventListener('error', (err) => {
                console.error('rpc连接出错:', err);
            });
        }
    } catch (e) {
        console.log("connection failed:", e);
        this.reconnect();
    }
};

Hlclient.prototype.reconnect = function () {
    console.log("10秒后尝试重连...");
    var _this = this;
    setTimeout(function () {
        _this.connect();
    }, 10000);
};

Hlclient.prototype.send = function (msg) {
    if (this.isWechat) {
        // 微信环境发送消息
        if (this.socket && this.socket.readyState === 1) {
            wx.sendSocketMessage({
                data: msg,
                fail(err) {
                    console.error('消息发送失败:', err);
                }
            });
        }
    } else {
        // 浏览器环境
        if (this.socket.readyState === WebSocket.OPEN) {
            this.socket.send(msg);
        }
    }
};

Hlclient.prototype.regAction = function (func_name, func) {
    if (typeof func_name !== 'string') throw new Error("func_name must be string");
    if (typeof func !== 'function') throw new Error("must be function");
    console.log("register func:", func_name);
    this.handlers[func_name] = func;
    return true;
};

Hlclient.prototype.handlerRequest = function (requestJson) {
    var _this = this;
    try {
        var result = JSON.parse(requestJson);
        // 微信环境持久化clientId
        if (result["registerId"]) {
            rpc_client_id = result['registerId'];
            if (this.isWechat) {
                wx.setStorageSync('rpc_client_id', rpc_client_id);
            }
            return;
        }

        if (!result['action'] || !result["message_id"]) {
            console.warn('Invalid request:', result);
            return;
        }

        var action = result["action"],
            message_id = result["message_id"],
            param = result["param"];

        try { param = JSON.parse(param); } catch (e) { }

        var handler = this.handlers[action];
        if (!handler) {
            return this.sendResult(action, message_id, 'Action not found');
        }

        handler(function (response) {
            _this.sendResult(action, message_id, response);
        }, param);

    } catch (error) {
        console.log("处理请求出错:", error);
        this.sendResult(action, message_id, error.message);
    }
};

Hlclient.prototype.sendResult = function (action, message_id, data) {
    if (typeof data === 'object') {
        try { data = JSON.stringify(data); } catch (e) { }
    }
    var response = JSON.stringify({
        action: action,
        message_id: message_id,
        response_data: data
    });
    this.send(response);
};
