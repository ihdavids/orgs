



var todos_section_loaded = (jrpc) => {
  //queryDay(jrpc, currentDay);
}

var todosInit = () => {
    for (var entry in todo_lists) {
        let el = document.getElementById("TodosList");
        if (el != null) {

            let node = document.createElement("a");
            node.className = "collapse-item";
            node.innerHTML = `${entry[0].toUpperCase()}${entry.slice(1)}`;
            let v = entry;

            node.onclick = () => { console.log(v); ShowTodos(v,todo_lists[v]); return false;};
            node.setAttribute('href',todo_lists[v])
            el.appendChild(node);
        }
        // <a class="collapse-item" onclick="ShowPage('todos_section');return false" href="buttons.html">Buttons</a>
        console.log(entry);
    }
}

/*
            <ul class="list-group list-group-horizontal rounded-0 bg-transparent">
              <li
                class="list-group-item d-flex align-items-center ps-0 pe-3 py-1 rounded-0 border-0 bg-transparent">
                <div class="form-check">
                  <input class="form-check-input me-0" type="checkbox" value="" id="flexCheckChecked1"
                    aria-label="..." checked />
                </div>
              </li>
              <li
                class="list-group-item px-3 py-1 d-flex align-items-center flex-grow-1 border-0 bg-transparent">
                <p class="lead fw-normal mb-0">Buy groceries for next week</p>
              </li>
              <li class="list-group-item ps-3 pe-0 py-1 rounded-0 border-0 bg-transparent">
                <div class="d-flex flex-row justify-content-end mb-1">
                  <a href="#!" class="text-info" data-mdb-toggle="tooltip" title="Edit todo"><i
                      class="fas fa-pencil-alt me-3"></i></a>
                  <a href="#!" class="text-danger" data-mdb-toggle="tooltip" title="Delete todo"><i
                      class="fas fa-trash-alt"></i></a>
                </div>
                <div class="text-end text-muted">
                  <a href="#!" class="text-muted" data-mdb-toggle="tooltip" title="Created date">
                    <p class="small mb-0"><i class="fas fa-info-circle me-2"></i>28th Jun 2020</p>
                  </a>
                </div>
              </li>
            </ul> 
*/

var statusMappings = {
  "TODO": '<i class="fas fa-expand me-0"></i>',
  "IN-PROGRESS": '<i class="fas fa-spinner me-0"></i>',
  "INPROGRESS": '<i class="fas fa-spinner me-0"></i>',
  "DOING": '<i class="fas fa-spinner me-0"></i>',
  "MEETING": '<i class="fas fa-clock me-0"></i>',
  "BLOCKED": '<i class="fas fa-radiation me-0"></i>',
  "WAITING": '<i class="fas fa-pause me-0"></i>',
  "PHONE": '<i class="fas fa-phone me-0"></i>',
  "NEXT": '<i class="fas fa-project-diagram me-0"></i>',

  "CANCELLED": '<i class="fas fa-times me-0"></i>',
  "DONE": '<i class="fas fa-check me-0"></i>',
  "UNDEFINED": '<i class="fas fa-slash me-0"></i>',
}


function ChangeStatus(to, item, activeStatus) {
	var query = [{'Hash': `${item["Hash"]}`, "Value": to}]
  jrpc.call('Db.ChangeStatus', query).then(function(res) {
    console.log("Got something back: " + JSON.stringify(res));
    if (res != null && res['result']["Ok"]) {
        item.Status   = to;
        item.IsActive = activeStatus;
    }
    RedrawTodos();
  });
}

function GetTodoHtml(item) {
  if ("ShowContent" in item && item.ShowContent == true) {
    item.ShowContent = false;
    RedrawTodos();
  } else {
	  var query = [`${item["Hash"]}`]
    jrpc.call('Db.QueryFullTodoHtml', query).then(function(res) {
      console.log("Got something back: " + JSON.stringify(res));
      if (res != null && res['result']["Content"]) {
        item["Content"] = res['result']["Content"];
        item["ShowContent"] = true;
      }
      RedrawTodos();
  });

  }
}

// Yes a module level variable. This is the currently active list.
var currentTodos = null;

function GetStatus(item) {
    let stat = item["Status"];
    let r = statusMappings[stat];
    if (r === undefined) {
        r = statusMappings["UNDEFINED"];
    }
    return r;
}

function addTodo(tlist, item, shouldBeActive) {
    let ul = document.createElement('ul');
    ul.className="list-group list-group-horizontal rounded-0 bg-transparent";
    tlist.appendChild(ul);

    let li = document.createElement('li');
    li.className ="list-group-item d-flex align-items-center ps-0 pe-3 py-1 rounded-0 border-0 bg-transparent";
    ul.appendChild(li);

    let dv = document.createElement('DIV');
    dv.className="form-check";
    li.appendChild(dv);

    /*
    let inpt = document.createElement("input");
    inpt.className="form-check-input me-0";
    inpt.type = "checkbox";
    inpt.value = "";
    //inpt.id = "";
    inpt.setAttribute('aria-label',"...");
    if (item["IsActive"]) {
      inpt.setAttribute('checked',value=null);
    }
    dv.appendChild(inpt);
*/
    let inpt = document.createElement("div");
    inpt.className="text-info me-0";

    if (item["IsActive"]) {
      inpt.innerHTML = GetStatus(item);
      inpt.onclick = () => {
        ChangeStatus("DONE",item, false);
      }
    } else {
      inpt.innerHTML = GetStatus(item);
      inpt.onclick = () => {
        ChangeStatus("TODO",item, true);
      }
    }
    dv.appendChild(inpt);

    // NAME
    li = document.createElement('li');
    li.className = "list-group-item px-3 py-1 d-flex align-items-center flex-grow-1 border-0 bg-transparent";
    ul.appendChild(li);
    
    p = document.createElement('p');
    p.className="lead fw-normal mb-0 red";
    if (shouldBeActive && !item.IsActive) {
        p.style['text-decoration'] = "line-through";
    }
   
    // TODO: This should open an HTML view of this node or file.
    //       Need to get creative on how to do that.
    p.innerHTML=item.Headline;
    p.onclick = () => {
      GetTodoHtml(item);
    }
    li.appendChild(p);

    // Controls
    li = document.createElement('li');
    li.className = "list-group-item ps-3 pe-0 py-1 rounded-0 border-0 bg-transparent";
    ul.appendChild(li);
   
    // Icons
    dv = document.createElement('DIV');
    dv.className="d-flex flex-row justify-content-end mb-1";
    li.appendChild(dv);

    a = document.createElement('a');
    a.className="text-info";
    a.title="Edit todo";
    a.setAttribute('data-mdb-toggle','tooltip');
    a.href="#!";
    a.innerHTML='<i class="fas fa-pencil-alt me-3"></i>';
    dv.appendChild(a);

    a = document.createElement('a');
    a.className="text-danger";
    a.title="Delete todo";
    a.setAttribute('data-mdb-toggle','tooltip');
    a.href="#!";
    a.innerHTML='<i class="fas fa-trash-alt"></i>';
    dv.appendChild(a);

    // Date
    dv = document.createElement('DIV');
    dv.className="text-end text-muted";
    li.appendChild(dv);
    
    a = document.createElement('a');
    a.className="text-muted";
    a.title="Created date";
    a.setAttribute('data-mdb-toggle','tooltip');
    a.href="#!";
    dv.appendChild(a);

    p = document.createElement('p');
    p.className="small mb-0";
    if (item.Tags != null) {
      p.innerHTML=`<i class="fas fa-info-circle me-2"></i>${item.Status} : ${item.Tags}`;
    } else {
      p.innerHTML=`<i class="fas fa-info-circle me-2"></i>${item.Status}`;
    }
    a.appendChild(p);

    if ("Content" in item && "ShowContent" in item && item.ShowContent == true) {
      ul = document.createElement('ul');
      ul.className="list-group list-group-horizontal rounded-0 bg-transparent";
      tlist.appendChild(ul);

      li = document.createElement('li');
      li.className = "list-group-item d-flex align-items-center ps-4 pe-5 py-1 rounded-0 border-0 bg-transparent";
      ul.appendChild(li);

      li = document.createElement('li');
      li.className = "list-group-item d-flex align-items-center ps-0 pe-3 py-1 rounded-0 border-0 bg-transparent";
      ul.appendChild(li);

      c = document.createElement('DIV');
      c.className = "card shadow";
      li.appendChild(c);

      dv = document.createElement('DIV');
      dv.className = "card-body py-4 px-md-5 org";
      dv.innerHTML = item["Content"];
      c.appendChild(dv);
    }
}

var RedrawTodos = () => {
    let numActive = 0;
    currentTodos.forEach( (item) => {
        if (item.IsActive) { numActive += 1; }
    });
    shouldBeActive = false;
    if (numActive > (currentTodos.length/4)) {
        shouldBeActive = true;
    }
    let tlist = document.getElementById('ActualTodoList');
    if (tlist != null) {
        tlist.innerHTML = '';
        currentTodos.forEach( (item) => {
            addTodo(tlist, item, shouldBeActive);
        });
    }
}

function LaunchEditor(item) {

}

var ShowTodos = (name,querystr) => {
    console.log(name);
    console.log(querystr);
    ShowPage("todos_section");
    let tit = document.getElementById('TodoListTitle');
    if (tit != null) {
        tit.innerHTML = name[0].toUpperCase() + name.slice(1);
    }

    let tlist = document.getElementById('ActualTodoList');
    if (tlist != null) {
        tlist.innerHTML = '';
        querystr = querystr.split("//")[0];
	    var query = [{'Query': `${querystr}`}]
        console.log("QUERY: " + JSON.stringify(query));
        jrpc.call('Db.QueryTodosExp', query).then(function(res) {
           console.log("Got something back: " + JSON.stringify(res));
           let events = [];
           if (res['result'] != null) {
            currentTodos = res['result'];
            RedrawTodos();
           //layOutDay(events);
           } else {
             //layOutDay(null);
            }
            //let el = document.getElementById("agendaTitle");
           //if (el != null) {
           //  el.innerHTML = "Agenda: " + wd + "     (" + yyyy + " " +  mm + " " +  dd + ")"
           //}
        });
    }


}