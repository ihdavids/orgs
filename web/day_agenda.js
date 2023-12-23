let containerHeight = 720;
let containerWidth = 600;
let collisions = [];
let width = [];
let leftOffSet = [];
let startHour = 9
let endHour   = 21
let minutesinDay = 60* (endHour - startHour)
let timeFormat = 12

function clamp(x) {
  if (x < 0) {
    return 0;
  }
  return x
}

var currentDay = new Date();

var calendar;

function show_calendar() {
calendar = createCalendar('.calendar', {
  type: 'month',
  date: {
    min: '2000-01-01',
    max: '2030-12-31',
    today: new Date('2022-01-07'),
  },
  settings: {
    lang: 'en',
    iso8601: true,
    range: {
      min: '2022-01-01',
      max: '2022-02-12',
      disabled: ['2022-01-25'],
    },
    selection: {
      day: 'multiple',
      month: false,
      year: false,
    },
    selected: {
      dates: ['2022-01-09', '2022-01-10'],
      month: 1,
      year: 2022,
      holidays: ['2022-01-02', '2022-01-03', '2022-01-04', '2022-01-05'],
    },
    visibility: {
      weekend: false,
      today: true,
      disabled: true,
    },
  },
  actions: {
    clickDay(e) {
      alert(e.target.dataset.calendarDay);
    },
    clickMonth(e) {
      alert(e.target.dataset.calendarMonth);
    },
    clickYear(e) {
      alert(e.target.dataset.calendarYear);
    },
  },
});

calendar.init();
}


var agenda_section_loaded = (jrpc) => {
  queryDay(jrpc, currentDay);
}

var showAgendaToday = () => {
  currentDay = new Date();
  queryDay(jrpc, currentDay);
}

function addDays(date, days) {
  var result = new Date(date);
  result.setDate(result.getDate() + days);
  return result;
}


var moveAgendaToTomorrow = () => {
  currentDay = addDays(currentDay, 1);
  queryDay(jrpc, currentDay);
}

var moveAgendaToYesterday = () => {
  currentDay = addDays(currentDay, -1);
  queryDay(jrpc, currentDay);
}

var queryDay = (jrpc, day) => {
  //show_calendar();
    var dd = String(day.getDate()).padStart(2, '0');
    var mm = String(day.getMonth() + 1).padStart(2, '0'); //January is 0!
    var yyyy = day.getFullYear();
    var wd = day.getDay();
    var wdlook = ["Sun","Mon","Tue","Wed","Thu","Fri","Sat"];
    wd = wdlook[wd];
    var myDate = yyyy + " " + dd + " " + mm;
    // REQUEST:  {"method":"Db.QueryTodosExp","params":[{"Query":"!IsArchived() \u0026\u0026 !IsProject() \u0026\u0026 IsTodo()"}],"id":5577006791947779410}
	  var query = [{'Query': `!IsProject() && !IsArchived() && IsTodo() && OnDate("${myDate}")`}]
    console.log("QUERY: " + JSON.stringify(query));
    jrpc.call('Db.QueryTodosExp', query).then(function(res) {
        console.log("Got something back: " + JSON.stringify(res));
        let events = [];
        if (res['result'] != null) {
        res['result'].forEach( (item) => {
          let s = new Date(item.Date.Start);
          let e = new Date(item.Date.End);
          events.push({headline: item.Headline, start: s, end: e});
        });
        layOutDay(events);
        } else {
          layOutDay(null);
        }
        let el = document.getElementById("agendaTitle");
        if (el != null) {
          el.innerHTML = "Agenda: " + wd + "     (" + yyyy + " " +  mm + " " +  dd + ")"
        }
    });
}



// append one event to calendar
var createEvent = (evt, height, top, left, units) => {

  let node = document.createElement("DIV");
  node.className = "agd-event";
  node.innerHTML = `<span class='agd-title'>${evt.headline}</span><br><span class='agd-location'> Sample Location </span>`;

  // Customized CSS to position each event
  node.style.width = (containerWidth/units) + "px";
  node.style.height = height + "px";
  node.style.top = top + "px";
  node.style.left = 100 + left + "px";

  //node.style['border-left-color'] = '#f00';

  document.getElementById("events").appendChild(node);
}

var createTimeMarker = (height, top, left, units) => {
  let node = document.createElement("DIV");
  let dot = document.createElement("DIV");
  dot.className = "agd-dot"
  node.className = "agd-timeMarker";

  // Customized CSS to position each event
  node.style.width = (containerWidth/units) + "px";
  node.style.height = height + "px";
  node.style.top = top + "px";
  node.style.left = left + "px";

  let r = height*4;
  dot.style.width = r + "px";
  dot.style.height = r + "px";
  dot.style.top = top - (r/2) + (height/2) + "px";
  dot.style.left = clamp(left - r/2) + "px";

  document.getElementById("events").appendChild(dot);
  document.getElementById("events").appendChild(node);
}

function getTimeBarWidth() {
  agd = document.getElementById("agenda");
  var timings = agd.querySelector('.agd-timings');
  return timings.clientWidth;
}

function createTimeBlocks() {
  agd = document.getElementById("agenda");
  var timings = agd.querySelector('.agd-timings');
  if (timings === null) {
    timings = document.createElement("DIV");
    timings.className = "agd-timings";
    agd.prepend(timings);
  }
  var days = agd.querySelector('.agd-days');
  if (days === null) {
    days = document.createElement("DIV");
    days.className = "agd-days";
    days.id = "events";

    agd.appendChild(days);
  }

  timings.innerHTML = '';
  for (let i = startHour; i <= endHour; ++i) {

    let node = document.createElement("DIV");
    let out = i
    if (timeFormat == 12) {
      suffix = " AM"
      if (i >= 12) {
        suffix = " PM"
      }
      if (i > 12) {
        out = i - 12
      }
      node.innerHTML = `<span>${out}:00</span>${suffix}` 
    } else {
      suffix = " Hrs"
      node.innerHTML = `<span>${out}:00</span>${suffix}`
    }

    timings.appendChild(node);


    if (i != endHour) {
      node = document.createElement("DIV");

      let out = i
      if (timeFormat == 12) {
        if (i > 12) {
          out = i - 12
        }
        node.innerHTML = `${out}:30` 
      } else {
        node.innerHTML = `${out}:30`
      }
      timings.appendChild(node);
    }
  }


}

/* 
collisions is an array that tells you which events are in each 30 min slot
- each first level of array corresponds to a 30 minute slot on the calendar 
  - [[0 - 30mins], [ 30 - 60mins], ...]
- next level of array tells you which event is present and the horizontal order
  - [0,0,1,2] 
  ==> event 1 is not present, event 2 is not present, event 3 is at order 1, event 4 is at order 2
*/

function getCollisions (events) {

  //resets storage
  collisions = [];
  if (events == null) {
    return;
  }

  for (var i = 0; i < 24; i ++) {
    var time = [];
    for (var j = 0; j < events.length; j++) {
      time.push(0);
    }
    collisions.push(time);
  }

  events.forEach((event, id) => {
    let end = getInMinutes(event.end);
    let start = getInMinutes(event.start);
    let order = 1;

    while (start < end) {
      timeIndex = Math.floor(start/30);

      while (order < events.length) {
        if (collisions[timeIndex].indexOf(order) === -1) {
          break;
        }
        order ++;
      }

      collisions[timeIndex][id] = order;
      start = start + 30;
    }

    collisions[Math.floor((end-1)/30)][id] = order;
  });
};

/*
find width and horizontal position

width - number of units to divide container width by
horizontal position - pixel offset from left
*/
function getAttributes (events) {

  //resets storage
  width = [];
  leftOffSet = [];

  if (events == null) {
    return;
  }

  for (var i = 0; i < events.length; i++) {
    width.push(0);
    leftOffSet.push(0);
  }

  collisions.forEach((period) => {

    // number of events in that period
    let count = period.reduce((a,b) => {
      return b ? a + 1 : a;
    })

    if (count > 1) {
      period.forEach((event, id) => {
        // max number of events it is sharing a time period with determines width
        if (period[id]) {
          if (count > width[id]) {
            width[id] = count;
          }
        }

        if (period[id] && !leftOffSet[id]) {
          leftOffSet[id] = period[id];
        }
      })
    }
  });
};

function getInMinutes(d) {
  startMinutes = startHour * 60;
  return (d.getHours() * 60 + d.getMinutes()) - startMinutes;
}

var layOutDay = (events) => {

  createTimeBlocks();
// clear any existing nodes
var myNode = document.getElementById("events");
myNode.innerHTML = '';

  getCollisions(events);
  getAttributes(events);

  var agd = document.getElementById("agenda");
  containerHeight = agd.clientHeight;
  //containerWidth  = agd.clientWidth;

  if (events != null) {
  events.forEach((event, id) => {
    console.log(event.start);
    console.log(event.end);
    let s = getInMinutes(event.start);
    let e = getInMinutes(event.end);
    let height = (e - s) / minutesinDay * containerHeight;
    let top = s / minutesinDay * containerHeight; 
    let units = width[id];
    if (!units) {units = 1};
    let left = (containerWidth / width[id]) * (leftOffSet[id] - 1) + 10;
    if (!left || left < 0) {left = 10};
    createEvent(event, height, top, left, units);
  });
  }

  const d = new Date();
  let s = getInMinutes(d);
  let height = 2;
  let top = s / minutesinDay * containerHeight; 
  //let units = width[id];
  //if (!units) {units = 1};
  units = 1;
  //let left = (containerWidth / width[id]) * (leftOffSet[id] - 1) + 10;
  //if (!left || left < 0) {left = 10};
  left = getTimeBarWidth();
  createTimeMarker(height, top, left, units);


  // Recompute the grid height for our agenda so the lines match our actual container height
  agd = document.getElementById("agenda");
  var days = agd.querySelector('.agd-days');
  var timings = agd.querySelector('.agd-days');
  let span = endHour - startHour;
  let totalHeight = timings.clientHeight;
  let blockHeight = totalHeight / span;
  days.style['background-size'] = "1px " + blockHeight + "px";

}