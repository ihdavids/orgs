var DayPage = () => {
    //QueryFullFileHtml
    //CreateDayPage
	var query = ['today'];
    jrpc.call('Db.CreateDayPage', query).then(function(res) {
      //console.log("Got something back: " + JSON.stringify(res));
      if (res != null && res['result'][0]) {
        let dpfn = res['result'][0];
        query = [dpfn]
        jrpc.call('Db.QueryFullFileHtml', query).then(function(res) {
            //console.log("Got something back: " + JSON.stringify(res));
            if ('result' in res && 'Content' in res['result']) {
                let content = res['result']['Content']
                //console.log(content);
                ShowPage('page_section');
                let el = document.getElementById('PageContent');
                el.innerHTML = content;
            }
        });
      }
    });
}