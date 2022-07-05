



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
            el.appendChild(node);
        }
        // <a class="collapse-item" onclick="ShowPage('todos_section');return false" href="buttons.html">Buttons</a>
        console.log(entry);
    }
}

var ShowTodos = (name,query) => {
    console.log(name);
    console.log(query);
    ShowPage("todos_section");
    let tit = document.getElementById('TodoListTitle');
    if (tit != null) {
        tit.innerHTML = name[0].toUpperCase() + name.slice(1);
    }
}