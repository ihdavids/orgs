
(function($) {
  "use strict"; // Start of use strict

  // Toggle the side navigation
  $("#sidebarToggle").on('click', function(e) {
    console.log("TRIED TO TOGGLE!");
    $("body").toggleClass("sidebar-toggled");
    $(".sidebar").toggleClass("toggled");
    if ($(".sidebar").hasClass("toggled")) {
      $('.sidebar .collapse').collapse('hide');
    };
  });

  // Close any open menu accordions when window is resized below 768px
  $(window).resize(function() {
    if ($(window).width() < 768) {
      $('.sidebar .collapse').collapse('hide');
    };
    
    // Toggle the side navigation when window is resized below 480px
    if ($(window).width() < 480 && !$(".sidebar").hasClass("toggled")) {
      $("body").addClass("sidebar-toggled");
      $(".sidebar").addClass("toggled");
      $('.sidebar .collapse').collapse('hide');
    };
  });

  // Prevent the content wrapper from scrolling when the fixed side navigation hovered over
  $('body.fixed-nav .sidebar').on('mousewheel DOMMouseScroll wheel', function(e) {
    if ($(window).width() > 768) {
      var e0 = e.originalEvent,
        delta = e0.wheelDelta || -e0.detail;
      this.scrollTop += (delta < 0 ? 1 : -1) * 30;
      e.preventDefault();
    }
  });

  // Scroll to top button appear
  $(document).on('scroll', function() {
    var scrollDistance = $(this).scrollTop();
    if (scrollDistance > 100) {
      $('.scroll-to-top').fadeIn();
    } else {
      $('.scroll-to-top').fadeOut();
    }
  });

  // Smooth scrolling using jQuery easing
  $(document).on('click', 'a.scroll-to-top', function(e) {
    var $anchor = $(this);
    $('html, body').stop().animate({
      scrollTop: ($($anchor.attr('href')).offset().top)
    }, 1000, 'easeInOutExpo');
    e.preventDefault();
  });

})(jQuery); // End of use strict

function escapeHtml(unsafe)
{
    return unsafe
         .replace(/&/g, "&amp;")
         .replace(/</g, "&lt;")
         .replace(/>/g, "&gt;")
         .replace(/"/g, "&quot;")
         .replace(/'/g, "&#039;");
 }

// Convert orgs default output to highlight JS style output.
function ResetLanguage() {
  $('[class*=" src-"]').each(function(idx) {
    let classList = $(this).attr("class");
    //console.log("SRC BLOCK");
    var classArr = classList.split(/\s+/);
    let langName = null;
    $.each(classArr, function(index, value){
      //console.log("VAL: ",value,value.indexOf("src-"));
      if (value.indexOf("src-") == 0) {
        langName = value.substr(4);
        //console.log("LANGNAME: ", langName);
      }
    });
    if (langName != null) {

      //console.log("LANGNAME: ", langName);
      let pres = $(this).children('.highlight').children('pre');
      let con = escapeHtml(pres.contents().text());
      pres.empty();
      //console.log("CONTENTS: " + con);
      let cod = document.createElement('code');
      cod.className = "language-" + langName;
      cod.innerHTML = con;
      pres.append(cod);
    }
  })
  hljs.highlightAll();
}

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
                console.log(content);
                ShowPage('page_section');
                let el = document.getElementById('PageContent');
                el.innerHTML = content;


                $('[id^=headline-]').on('click',function(e) {
                    let id = $(this).attr('id');
                    let divid="#outline-text-" + id;
                    console.log(divid);
                    $(divid).toggle();
                });
                ResetLanguage();
            }
        });
      }
    });
}