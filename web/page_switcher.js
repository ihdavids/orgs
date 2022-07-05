let currentPage = "agenda-section";
let psJrpc = null;

function SetPageSwitcherJrpc(jjj) {
    psJrpc = jjj;
}

function ShowPage(name) {
    console.log("SHOW PAGE: " + name + " FROM: " + currentPage)
   let nel = document.getElementById(name);
   let el = document.getElementById(currentPage);
   if (nel != null) {
    if (el != null) {
        el.style.display = "none";
    }
    nel.style.display = "block";

    currentPage = name;

    // Call the loaded method on this section if present.
    loadedName = name + "_loaded";
    if (loadedName in window) {
        window[loadedName](psJrpc);
    }
   }
}

