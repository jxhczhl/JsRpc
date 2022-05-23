hlc = new Hlclient("ws://127.0.0.1:12080/ws?group=zzz&name=hlg");


hlc.regAction("hello", function (resolve,param) {
    var base666 = btoa(param)
    resolve(base666 + "**" + atob(base666));

})