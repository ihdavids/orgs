let currentPage = "agenda-section";


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
   }
}