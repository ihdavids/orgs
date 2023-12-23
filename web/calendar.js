var createCalendar = (selector, option) => {
    let cal = new Object();
		cal.HTMLElement = document.querySelector(selector);
		if (!cal.HTMLElement) return null;
		cal.type = option?.type ?? 'default';
		cal.date = {
			min: option?.date?.min ?? '1970-01-01',
			max: option?.date?.max ?? '2470-12-31',
			today: option?.date?.today ?? new Date(),
		};
		cal.settings = {
			lang: option?.settings?.lang ?? 'en',
			iso8601: option?.settings?.iso8601 ?? true,
			range: {
				min: option?.settings?.range?.min ?? cal.date.min,
				max: option?.settings?.range?.max ?? cal.date.max,
				disabled: option?.settings?.range?.disabled ?? null,
			},
			selection: {
				day: option?.settings?.selection?.day ?? 'single',
				month: option?.settings?.selection?.month ?? true,
				year: option?.settings?.selection?.year ?? true,
			},
			selected: {
				dates: option?.settings?.selected?.dates ?? null,
				month: option?.settings?.selected?.month ?? null,
				year: option?.settings?.selected?.year ?? null,
				holidays: option?.settings?.selected?.holidays ?? null,
			},
			visibility: {
				weekend: option?.settings?.visibility?.weekend ?? true,
				today: option?.settings?.visibility?.today ?? true,
				disabled: option?.settings?.visibility?.disabled ?? false,
			},
		};
		cal.actions = {
			clickDay: option?.actions?.clickDay ?? null,
			clickMonth: option?.actions?.clickMonth ?? null,
			clickYear: option?.actions?.clickYear ?? null,
		};
		cal.name = {
			months: {
				full: {
					en: ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'],
					ru: ['Январь', 'Февраль', 'Март', 'Апрель', 'Май', 'Июнь', 'Июль', 'Август', 'Сентябрь', 'Октябрь', 'Ноябрь', 'Декабрь'],
				},
				reduction: {
					en: ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'],
					ru: ['Янв', 'Фев', 'Мар', 'Апр', 'Май', 'Июн', 'Июл', 'Авг', 'Сен', 'Окт', 'Ноя', 'Дек'],
				},
			},
			week: {
				en: ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'],
				ru: ['Вс', 'Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб'],
			},
			arrow: {
				prev: {
					en: 'Prev',
					ru: 'Назад',
				},
				next: {
					en: 'Next',
					ru: 'Вперед',
				},
			},
		};

		cal.currentType = cal.type;
	};

	cal.setVariablesDates = () => {
		cal.selectedDates = [];
		cal.selectedMonth = cal.date.today.getUTCMonth();
		cal.selectedYear = cal.date.today.getUTCFullYear();

		if (cal.settings.selected.dates !== null) {
			cal.selectedDates = cal.settings.selected.dates;
		}

		if (cal.settings.selected.month !== null && cal.settings.selected.month >= 0 && cal.settings.selected.month < 12) {
			cal.selectedMonth = cal.settings.selected.month;
		}

		if (cal.settings.selected.year !== null && cal.settings.selected.year >= 0 && cal.settings.selected.year <= 9999) {
			cal.selectedYear = cal.settings.selected.year;
		}

		cal.viewYear = cal.selectedYear;
		cal.dateMin = cal.settings.visibility.disabled ? new Date(cal.date.min) : new Date(cal.settings.range.min);
		cal.dateMax = cal.settings.visibility.disabled ? new Date(cal.date.max) : new Date(cal.settings.range.max);
	};

	cal.createDOM = () => {
		if (cal.currentType === 'default') {
			cal.HTMLElement.classList.add('vanilla-calendar_default');
			cal.HTMLElement.classList.remove('vanilla-calendar_month');
			cal.HTMLElement.classList.remove('vanilla-calendar_year');
			cal.HTMLElement.innerHTML = `
			<div class="vanilla-calendar-header">
				<button type="button"
					class="vanilla-calendar-arrow vanilla-calendar-arrow_prev">
					${cal.name.arrow.prev[cal.settings.lang] ?? cal.name.arrow.prev.en}
				</button>
				<div class="vanilla-calendar-header__content">
					<b class="vanilla-calendar-month${cal.settings.selection.month ? '' : ' vanilla-calendar-month_disabled'}"></b>
					<b class="vanilla-calendar-year${cal.settings.selection.year ? '' : ' vanilla-calendar-year_disabled'}"></b>
				</div>
				<button type="button"
					class="vanilla-calendar-arrow vanilla-calendar-arrow_next">
					${cal.name.arrow.next[cal.settings.lang] ?? cal.name.arrow.next.en}
				</button>
			</div>
			<div class="vanilla-calendar-content">
				<div class="vanilla-calendar-week"></div>
				<div class="vanilla-calendar-days"></div>
			</div>
		`;
		} else if (cal.currentType === 'month') {
			cal.HTMLElement.classList.remove('vanilla-calendar_default');
			cal.HTMLElement.classList.add('vanilla-calendar_month');
			cal.HTMLElement.classList.remove('vanilla-calendar_year');
			cal.HTMLElement.innerHTML = `
			<div class="vanilla-calendar-header">
				<button type="button"
					class="vanilla-calendar-arrow vanilla-calendar-arrow_prev"
					style="visibility: hidden">
					${cal.name.arrow.prev[cal.settings.lang] ?? cal.name.arrow.prev.en}
				</button>
				<div class="vanilla-calendar-header__content">
					<b class="vanilla-calendar-month"></b>
					<b class="vanilla-calendar-year vanilla-calendar-year_not-active${cal.settings.selection.year ? '' : ' vanilla-calendar-year_disabled'}"></b>
				</div>
				<button type="button"
					class="vanilla-calendar-arrow vanilla-calendar-arrow_next"
					style="visibility: hidden">
					${cal.name.arrow.next[cal.settings.lang] ?? cal.name.arrow.next.en}
				</button>
			</div>
			<div class="vanilla-calendar-content">
				<div class="vanilla-calendar-months"></div>
			</div>`;
		} else if (cal.currentType === 'year') {
			cal.HTMLElement.classList.remove('vanilla-calendar_default');
			cal.HTMLElement.classList.remove('vanilla-calendar_month');
			cal.HTMLElement.classList.add('vanilla-calendar_year');
			cal.HTMLElement.innerHTML = `
			<div class="vanilla-calendar-header">
				<button type="button"
					class="vanilla-calendar-arrow vanilla-calendar-arrow_prev">
					${cal.name.arrow.prev[cal.settings.lang] ?? cal.name.arrow.prev.en}
				</button>
				<div class="vanilla-calendar-header__content">
					<b class="vanilla-calendar-month vanilla-calendar-month_not-active${cal.settings.selection.month ? '' : ' vanilla-calendar-month_disabled'}"></b>
					<b class="vanilla-calendar-year"></b>
				</div>
				<button type="button"
					class="vanilla-calendar-arrow vanilla-calendar-arrow_next">
					${cal.name.arrow.next[cal.settings.lang] ?? cal.name.arrow.next.en}
				</button>
			</div>
			<div class="vanilla-calendar-content">
				<div class="vanilla-calendar-years"></div>
			</div>`;
		}
	};

	cal.generateDate = (date) => {
		const year = date.getUTCFullYear();
		let month = date.getUTCMonth() + 1;
		let day = date.getUTCDate();

		month = month < 10 ? `0${month}` : month;
		day = day < 10 ? `0${day}` : day;

		return `${year}-${month}-${day}`;
	};

	cal.controlArrows = () => {
		if (!['default', 'year'].includes(cal.currentType)) return;

		const arrowPrev = cal.HTMLElement.querySelector('.vanilla-calendar-arrow_prev');
		const arrowNext = cal.HTMLElement.querySelector('.vanilla-calendar-arrow_next');

		const defaultControl = () => {
			if (cal.currentType !== 'default') return;

			const isSelectedMinMount = cal.selectedMonth === cal.dateMin.getUTCMonth();
			const isSelectedMaxMount = cal.selectedMonth === cal.dateMax.getUTCMonth();
			const isSelectedMinYear = cal.selectedYear === cal.dateMin.getUTCFullYear();
			const isSelectedMaxYear = cal.selectedYear === cal.dateMax.getUTCFullYear();

			if ((isSelectedMinMount && isSelectedMinYear) || !cal.settings.selection.month) {
				arrowPrev.style.visibility = 'hidden';
			} else {
				arrowPrev.style.visibility = null;
			}
			if ((isSelectedMaxMount && isSelectedMaxYear) || !cal.settings.selection.month) {
				arrowNext.style.visibility = 'hidden';
			} else {
				arrowNext.style.visibility = null;
			}
		};

		const yearControl = () => {
			if (cal.currentType !== 'year') return;

			if (cal.dateMin.getUTCFullYear() && (cal.viewYear - 7) <= cal.dateMin.getUTCFullYear()) {
				arrowPrev.style.visibility = 'hidden';
			} else {
				arrowPrev.style.visibility = null;
			}

			if (cal.dateMax.getUTCFullYear() && (cal.viewYear + 7) >= cal.dateMax.getUTCFullYear()) {
				arrowNext.style.visibility = 'hidden';
			} else {
				arrowNext.style.visibility = null;
			}
		};

		defaultControl();
		yearControl();
	};

	cal.writeYear = () => {
		const yearEl = cal.HTMLElement.querySelector('.vanilla-calendar-year');
		yearEl.innerText = cal.selectedYear;
	};

	cal.writeMonth = () => {
		const monthEl = cal.HTMLElement.querySelector('.vanilla-calendar-month');
		monthEl.innerText = cal.name.months.full[cal.settings.lang][cal.selectedMonth];
	};

	cal.createWeek = () => {
		const weekEl = cal.HTMLElement.querySelector('.vanilla-calendar-week');

		const week = [...cal.name.week[cal.settings.lang]];
		if (cal.settings.iso8601) week.push(week.shift());

		for (let i = 0; i < week.length; i++) {
			const weekDayName = week[i];
			const weekDay = document.createElement('span');

			weekDay.className = 'vanilla-calendar-week__day';

			if (cal.settings.visibility.weekend && cal.settings.iso8601) {
				if (i === 5 || i === 6) {
					weekDay.classList.add('vanilla-calendar-week__day_weekend');
				}
			} else if (cal.settings.visibility.weekend && !cal.settings.iso8601) {
				if (i === 0 || i === 6) {
					weekDay.classList.add('vanilla-calendar-week__day_weekend');
				}
			}

			weekDay.innerText = `${weekDayName}`;
			weekEl.append(weekDay);
		}
	};

	cal.createDays = () => {
		const firstDay = new Date(Date.UTC(cal.selectedYear, cal.selectedMonth, 1));
		const daysSelectedMonth = new Date(Date.UTC(cal.selectedYear, cal.selectedMonth + 1, 0)).getUTCDate();

		let firstDayWeek = Number(firstDay.getUTCDay());
		if (cal.settings.iso8601) firstDayWeek = Number((firstDay.getUTCDay() !== 0 ? firstDay.getUTCDay() : 7) - 1);

		const daysEl = cal.HTMLElement.querySelector('.vanilla-calendar-days');
		if (['single', 'multiple', 'multiple-ranged'].includes(cal.settings.selection.day)) {
			daysEl.classList.add('vanilla-calendar-days_selecting');
		}

		daysEl.innerHTML = '';

		const setDayModifier = (dayEl, dayID, date) => {
			// if weekend
			if (cal.settings.visibility.weekend && (dayID === 0 || dayID === 6)) {
				dayEl.classList.add('vanilla-calendar-day_weekend');
			}

			// if holidays
			if (Array.isArray(cal.settings.selected.holidays)) {
				cal.settings.selected.holidays.forEach((holiday) => {
					if (holiday === date) {
						dayEl.classList.add('vanilla-calendar-day_holiday');
					}
				});
			}

			// if today
			let thisToday = cal.date.today.getUTCDate();
			let thisMonth = cal.date.today.getUTCMonth() + 1;
			thisToday = thisToday < 10 ? `0${thisToday}` : thisToday;
			thisMonth = thisMonth < 10 ? `0${thisMonth}` : thisMonth;

			const thisDay = `${cal.date.today.getUTCFullYear()}-${thisMonth}-${thisToday}`;

			if (cal.settings.visibility.today && dayEl.dataset.calendarDay === thisDay) {
				dayEl.classList.add('vanilla-calendar-day_today');
			}

			// if selected day
			if (cal.selectedDates.find((selectedDate) => selectedDate === date)) {
				dayEl.classList.add('vanilla-calendar-day_selected');
			}

			// if range min/max
			if (cal.settings.range.min > date || cal.settings.range.max < date) {
				dayEl.classList.add('vanilla-calendar-day_disabled');
			}

			// if range values
			if (Array.isArray(cal.settings.range.disabled)) {
				cal.settings.range.disabled.forEach((dateDisabled) => {
					if (dateDisabled === date) {
						dayEl.classList.add('vanilla-calendar-day_disabled');
					}
				});
			}
		};

		const prevMonth = () => {
			const prevMonthDays = new Date(Date.UTC(cal.selectedYear, cal.selectedMonth, 0)).getUTCDate();
			let day = prevMonthDays - firstDayWeek;
			let year = cal.selectedYear;
			let month = cal.selectedMonth;

			if (cal.selectedMonth === 0) {
				month = cal.name.months.full[cal.settings.lang].length;
				year = cal.selectedYear - 1;
			} else if (cal.selectedMonth < 10) {
				month = `0${cal.selectedMonth}`;
			}

			for (let i = 0; i < firstDayWeek; i++) {
				const dayEl = document.createElement('span');

				day += 1;

				const date = `${year}-${month}-${day}`;
				const dayIDCurrent = new Date(Date.UTC(cal.selectedYear, cal.selectedMonth, day - 1));
				const prevMonthID = dayIDCurrent.getUTCMonth() - 1;
				const dayID = new Date(Date.UTC(cal.selectedYear, prevMonthID, day)).getUTCDay();

				dayEl.className = 'vanilla-calendar-day vanilla-calendar-day_prev';
				dayEl.innerText = `${day}`;
				dayEl.dataset.calendarDay = date;

				setDayModifier(dayEl, dayID, date);
				daysEl.append(dayEl);
			}
		};

		const selectedMonth = () => {
			for (let i = 1; i <= daysSelectedMonth; i++) {
				const dayEl = document.createElement('span');
				const day = new Date(Date.UTC(cal.selectedYear, cal.selectedMonth, i));

				const date = cal.generateDate(day);
				const dayID = day.getUTCDay();

				dayEl.className = 'vanilla-calendar-day';
				dayEl.innerText = `${i}`;
				dayEl.dataset.calendarDay = date;

				setDayModifier(dayEl, dayID, date);
				daysEl.append(dayEl);
			}
		};

		const nextMonth = () => {
			const total = firstDayWeek + daysSelectedMonth;
			const rows = Math.ceil(total / cal.name.week[cal.settings.lang].length);
			const nextDays = (cal.name.week[cal.settings.lang].length * rows) - total;

			let year = cal.selectedYear;
			let month = cal.selectedMonth + 2;

			if ((cal.selectedMonth + 1) === cal.name.months.full[cal.settings.lang].length) {
				month = '01';
				year = cal.selectedYear + 1;
			} else if ((cal.selectedMonth + 2) < 10) {
				month = `0${cal.selectedMonth + 2}`;
			}

			for (let i = 1; i <= nextDays; i++) {
				const dayEl = document.createElement('span');
				const day = i < 10 ? `0${i}` : i;

				const date = `${year}-${month}-${day}`;
				const dayIDCurrent = new Date(Date.UTC(cal.selectedYear, cal.selectedMonth, i));
				const nextMonthID = dayIDCurrent.getUTCMonth() + 1;
				const dayID = new Date(Date.UTC(cal.selectedYear, nextMonthID, i)).getUTCDay();

				dayEl.className = 'vanilla-calendar-day vanilla-calendar-day_next';
				dayEl.innerText = `${i}`;
				dayEl.dataset.calendarDay = date;

				setDayModifier(dayEl, dayID, date);
				daysEl.append(dayEl);
			}
		};

		prevMonth();
		selectedMonth();
		nextMonth();
	};

	cal.changeMonth = (element) => {
		const lastMonth = cal.name.months.full[cal.settings.lang].length - 1;

		if (element.closest('.vanilla-calendar-arrow_prev')) {
			if (cal.selectedMonth !== 0) {
				cal.selectedMonth -= 1;
			} else if (cal.settings.selection.year) {
				cal.selectedYear -= 1;
				cal.selectedMonth = lastMonth;
			}
		} else if (element.closest('.vanilla-calendar-arrow_next')) {
			if (cal.selectedMonth !== lastMonth) {
				cal.selectedMonth += 1;
			} else if (cal.settings.selection.year) {
				cal.selectedYear += 1;
				cal.selectedMonth = 0;
			}
		}

		cal.settings.selected.month = cal.selectedMonth;

		cal.controlArrows();
		cal.writeYear();
		cal.writeMonth();
		cal.createDays();
	};

	cal.createYears = () => {
		cal.currentType = 'year';
		cal.createDOM();
		cal.controlArrows();
		cal.writeYear();
		cal.writeMonth();

		const yearsEl = cal.HTMLElement.querySelector('.vanilla-calendar-years');
		if (cal.settings.selection.year) yearsEl.classList.add('vanilla-calendar-years_selecting');

		for (let i = cal.viewYear - 7; i < cal.viewYear + 8; i++) {
			const year = i;
			const yearEl = document.createElement('span');
			yearEl.className = 'vanilla-calendar-years__year';

			if (year === cal.selectedYear) {
				yearEl.classList.add('vanilla-calendar-years__year_selected');
			}
			if (year < cal.dateMin.getUTCFullYear()) {
				yearEl.classList.add('vanilla-calendar-years__year_disabled');
			}
			if (year > cal.dateMax.getUTCFullYear()) {
				yearEl.classList.add('vanilla-calendar-years__year_disabled');
			}

			yearEl.dataset.calendarYear = year;
			yearEl.innerText = `${year}`;
			yearsEl.append(yearEl);
		}
	};

	cal.createMonths = () => {
		cal.currentType = 'month';
		cal.createDOM();
		cal.writeYear();
		cal.writeMonth();

		const monthsEl = cal.HTMLElement.querySelector('.vanilla-calendar-months');
		if (cal.settings.selection.month) monthsEl.classList.add('vanilla-calendar-months_selecting');

		const months = cal.name.months.reduction[cal.settings.lang];

		for (let i = 0; i < months.length; i++) {
			const month = months[i];
			const monthEl = document.createElement('span');

			monthEl.className = 'vanilla-calendar-months__month';

			if (i === cal.selectedMonth) {
				monthEl.classList.add('vanilla-calendar-months__month_selected');
			}
			if (i < cal.dateMin.getUTCMonth() && cal.selectedYear === cal.dateMin.getUTCFullYear()) {
				monthEl.classList.add('vanilla-calendar-months__month_disabled');
			}
			if (i > cal.dateMax.getUTCMonth() && cal.selectedYear === cal.dateMax.getUTCFullYear()) {
				monthEl.classList.add('vanilla-calendar-months__month_disabled');
			}

			monthEl.dataset.calendarMonth = i;

			monthEl.innerText = `${month}`;
			monthsEl.append(monthEl);
		}
	};

	cal.update = () => {
		cal.setVariablesDates();
		cal.createDOM();
		cal.controlArrows();
		cal.writeYear();
		cal.writeMonth();
		if (cal.currentType === 'default') {
			cal.createWeek();
			cal.createDays();
		} else if (cal.currentType === 'month') {
			cal.createMonths();
		} else if (cal.currentType === 'year') {
			cal.createYears();
		}
	};

	cal.click = () => {
		cal.HTMLElement.addEventListener('click', (e) => {
			const arrowEl = e.target.closest('.vanilla-calendar-arrow');
			const arrowPrevEl = e.target.closest('.vanilla-calendar-arrow_prev');
			const arrowNextEl = e.target.closest('.vanilla-calendar-arrow_next');
			const dayEl = e.target.closest('.vanilla-calendar-day');
			const dayPrevEl = e.target.closest('.vanilla-calendar-day_prev');
			const dayNextEl = e.target.closest('.vanilla-calendar-day_next');
			const yearHeaderEl = e.target.closest('.vanilla-calendar-year');
			const yearItemEl = e.target.closest('.vanilla-calendar-years__year');
			const monthHeaderEl = e.target.closest('.vanilla-calendar-month');
			const monthItemEl = e.target.closest('.vanilla-calendar-months__month');

			const clickDaySingle = () => {
				if (dayEl.classList.contains('vanilla-calendar-day_selected')) {
					cal.selectedDates.splice(cal.selectedDates.indexOf(dayEl.dataset.calendarDay), 1);
				} else {
					cal.selectedDates = [];
					cal.selectedDates.push(dayEl.dataset.calendarDay);
				}
			};

			const clickDayMultiple = () => {
				if (dayEl.classList.contains('vanilla-calendar-day_selected')) {
					cal.selectedDates.splice(cal.selectedDates.indexOf(dayEl.dataset.calendarDay), 1);
				} else {
					cal.selectedDates.push(dayEl.dataset.calendarDay);
				}
			};

			const clickDayMultipleRanged = () => {
				if (cal.selectedDates.length > 1) cal.selectedDates = [];
				cal.selectedDates.push(dayEl.dataset.calendarDay);

				if (!cal.selectedDates[1]) return;

				const startDate = new Date(Date.UTC(
					new Date(cal.selectedDates[0]).getUTCFullYear(),
					new Date(cal.selectedDates[0]).getUTCMonth(),
					new Date(cal.selectedDates[0]).getUTCDate(),
				));

				const endDate = new Date(Date.UTC(
					new Date(cal.selectedDates[1]).getUTCFullYear(),
					new Date(cal.selectedDates[1]).getUTCMonth(),
					new Date(cal.selectedDates[1]).getUTCDate(),
				));

				const addSelectedDate = (day) => {
					const date = cal.generateDate(day);
					if (cal.settings.range.disabled && cal.settings.range.disabled.includes(date)) return;
					cal.selectedDates.push(date);
				};

				cal.selectedDates = [];

				if (endDate > startDate) {
					for (let i = startDate; i <= endDate; i.setUTCDate(i.getUTCDate() + 1)) {
						addSelectedDate(i);
					}
				} else {
					for (let i = startDate; i >= endDate; i.setUTCDate(i.getUTCDate() - 1)) {
						addSelectedDate(i);
					}
				}
			};

			const clickDay = () => {
				if (['single', 'multiple', 'multiple-ranged'].includes(cal.settings.selection.day) && dayEl) {
					if (!dayPrevEl && !dayNextEl) {
						switch (cal.settings.selection.day) {
							case 'single':
								clickDaySingle();
								break;
							case 'multiple':
								clickDayMultiple();
								break;
							case 'multiple-ranged':
								clickDayMultipleRanged();
								break;
							// no default
						}

						if (cal.actions.clickDay) cal.actions.clickDay(e);
						cal.settings.selected.dates = cal.selectedDates;
						cal.createDays();
					}
				} else if (arrowEl && cal.currentType !== 'year' && cal.currentType !== 'month') {
					cal.changeMonth(e.target);
				}
			};

			const clickYear = () => {
				if (!cal.settings.selection.year) return;
				if (arrowEl && cal.currentType === 'year') {
					if (arrowNextEl) {
						cal.viewYear += 15;
					} else if (arrowPrevEl) {
						cal.viewYear -= 15;
					}
					cal.createYears();
				} else if (cal.currentType !== 'year' && yearHeaderEl) {
					cal.createYears();
				} else if (cal.currentType === 'year' && yearHeaderEl) {
					cal.currentType = cal.type;
					cal.update();
				} else if (yearItemEl) {
					const year = Number(yearItemEl.dataset.calendarYear);
					cal.currentType = cal.type;
					if (cal.selectedMonth < cal.dateMin.getUTCMonth() && year === cal.dateMin.getUTCFullYear()) {
						cal.settings.selected.month = cal.dateMin.getUTCMonth();
					}
					if (cal.selectedMonth > cal.dateMax.getUTCMonth() && year === cal.dateMax.getUTCFullYear()) {
						cal.settings.selected.month = cal.dateMax.getUTCMonth();
					}
					cal.settings.selected.year = year;
					if (cal.actions.clickYear) cal.actions.clickYear(e);
					cal.update();
				}
			};

			const clickMonth = () => {
				if (!cal.settings.selection.month) return;
				if (cal.currentType !== 'month' && monthHeaderEl) {
					cal.createMonths();
				} else if (cal.currentType === 'month' && monthHeaderEl) {
					cal.currentType = cal.type;
					cal.update();
				} else if (monthItemEl) {
					const month = Number(monthItemEl.dataset.calendarMonth);
					cal.currentType = cal.type;
					cal.settings.selected.month = month;
					if (cal.actions.clickMonth) cal.actions.clickMonth(e);
					cal.update();
				}
			};

			clickDay();
			clickYear();
			clickMonth();
		});
	};

	cal.init = () => {
		if (!cal.HTMLElement) return;
		cal.update();
		cal.click();
	};

    return cal;
}
