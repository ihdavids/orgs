<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{ sheet.name }} - Character Sheet</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Cinzel:wght@500;700&family=EB+Garamond:ital,wght@0,400;0,600;1,400&display=swap">
<style>
:root {
  --ink: #1c1a17;
  --paper: #f4efe4;
  --paper-2: #ece4d5;
  --line: #cbbfa6;
  --edge: #8b7e66;
  --accent: #7b1b1b;
  --accent-2: #b8860b;
  --muted: #6b6355;
  --shadow: 0 1px 3px rgba(28,26,23,.18);
}
* { box-sizing: border-box; }
html { -webkit-text-size-adjust: 100%; }
body {
  margin: 0;
  background: #2b2723;
  color: var(--ink);
  font-family: "EB Garamond", Palatino, "Palatino Linotype", Georgia, serif;
  font-size: 15px;
  line-height: 1.45;
}
.page {
  max-width: 1180px;
  margin: 0 auto;
  padding: 22px;
  background: var(--paper);
  background-image:
    radial-gradient(circle at 20% 10%, rgba(184,134,11,.06), transparent 45%),
    radial-gradient(circle at 85% 70%, rgba(123,27,27,.05), transparent 40%);
  box-shadow: 0 0 40px rgba(0,0,0,.55);
  min-height: 100vh;
}
h1, h2, h3, .label, .tile-label, th {
  font-family: Cinzel, "Trajan Pro", Georgia, serif;
  letter-spacing: .06em;
}

/* ---------------- header ---------------- */
header.sheet-head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 18px;
  border-bottom: 3px double var(--accent);
  padding-bottom: 12px;
  margin-bottom: 18px;
}
.name-block { flex: 1 1 320px; }
.char-name {
  font-size: 2.6rem;
  margin: 0;
  line-height: 1;
  color: var(--accent);
  text-shadow: 0 1px 0 rgba(255,255,255,.6);
}
.char-sub { color: var(--muted); font-size: 1.05rem; margin-top: 4px; }
.head-facts { display: flex; flex-wrap: wrap; gap: 10px; }
.fact {
  background: var(--paper-2);
  border: 1px solid var(--line);
  border-radius: 4px;
  padding: 5px 12px;
  min-width: 92px;
  box-shadow: var(--shadow);
}
.fact .label { display: block; font-size: .62rem; text-transform: uppercase; color: var(--muted); }
.fact .value { font-size: 1.05rem; font-weight: 600; }

/* ---------------- layout ---------------- */
.columns {
  display: grid;
  grid-template-columns: 250px 1fr 1fr;
  gap: 18px;
  align-items: start;
}
@media (max-width: 1000px) { .columns { grid-template-columns: 1fr 1fr; } }
@media (max-width: 700px)  { .columns { grid-template-columns: 1fr; } }
.col { display: flex; flex-direction: column; gap: 16px; min-width: 0; }

.box {
  background: var(--paper-2);
  border: 1px solid var(--line);
  border-radius: 5px;
  box-shadow: var(--shadow);
  padding: 10px 12px 12px;
}
.box > h2 {
  font-size: .74rem;
  text-transform: uppercase;
  margin: 0 0 8px;
  padding-bottom: 5px;
  border-bottom: 1px solid var(--line);
  color: var(--accent);
}

/* ---------------- abilities ---------------- */
.abilities { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; }
@media (max-width: 1000px) { .abilities { grid-template-columns: repeat(6, 1fr); } }
@media (max-width: 600px)  { .abilities { grid-template-columns: repeat(3, 1fr); } }
.ability {
  background: var(--paper);
  border: 2px solid var(--line);
  border-radius: 10px;
  text-align: center;
  padding: 8px 2px 4px;
  position: relative;
}
.ability .tile-label { font-size: .6rem; text-transform: uppercase; color: var(--muted); }
.ability .mod { font-size: 1.7rem; font-weight: 700; line-height: 1.05; }
.ability .score {
  display: inline-block;
  margin-top: 2px;
  min-width: 34px;
  border: 1px solid var(--line);
  border-radius: 12px;
  background: var(--paper-2);
  font-size: .8rem;
  padding: 0 6px;
}

/* ---------------- lists ---------------- */
.rows { display: flex; flex-direction: column; gap: 2px; }
.row {
  display: flex;
  align-items: baseline;
  gap: 7px;
  padding: 1px 2px;
  border-bottom: 1px dotted rgba(139,126,102,.45);
  font-size: .92rem;
}
.row:last-child { border-bottom: 0; }
.pip {
  width: 11px; height: 11px;
  border: 1px solid var(--muted);
  border-radius: 50%;
  flex: 0 0 auto;
  background: var(--paper);
}
.pip.on { background: var(--ink); }
.pip.exp { background: var(--accent-2); box-shadow: inset 0 0 0 2px var(--paper); }
.row .val { width: 2.4em; text-align: right; font-weight: 600; }
.row .abbr { color: var(--muted); font-size: .72rem; width: 2.6em; }
.row .nm { flex: 1; }

/* ---------------- combat tiles ---------------- */
.combat { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; }
.tile {
  background: var(--paper);
  border: 2px solid var(--line);
  border-radius: 6px;
  text-align: center;
  padding: 6px 4px;
}
.tile .tile-label { display: block; font-size: .58rem; text-transform: uppercase; color: var(--muted); }
.tile .big { font-size: 1.5rem; font-weight: 700; line-height: 1.15; }
.tile .sub { font-size: .68rem; color: var(--muted); }
.shield {
  border-radius: 6px 6px 26px 26px / 6px 6px 34px 34px;
  border-color: var(--accent);
}
.hp-bar {
  height: 9px; border-radius: 5px; background: #d8cbb2;
  border: 1px solid var(--line); overflow: hidden; margin-top: 6px;
}
.hp-fill { height: 100%; background: linear-gradient(90deg, #7b1b1b, #a33); }

/* ---------------- tables ---------------- */
table { width: 100%; border-collapse: collapse; font-size: .88rem; }
th {
  text-align: left; font-size: .6rem; text-transform: uppercase;
  color: var(--muted); border-bottom: 1px solid var(--line); padding: 2px 4px;
  font-weight: 600;
}
td { padding: 2px 4px; border-bottom: 1px dotted rgba(139,126,102,.4); vertical-align: top; }
tr:last-child td { border-bottom: 0; }
td.num, th.num { text-align: right; white-space: nowrap; }
.table-wrap { overflow-x: auto; }

/* ---------------- features & text ---------------- */
.feature { margin-bottom: 9px; }
.feature h3 {
  font-size: .82rem; margin: 0 0 1px; color: var(--accent);
}
.feature .src { font-size: .62rem; color: var(--muted); text-transform: uppercase; }
.feature p { margin: 2px 0; font-size: .86rem; white-space: pre-wrap; }
.scroller { max-height: 900px; overflow-y: auto; padding-right: 6px; }
.quote { font-style: italic; white-space: pre-wrap; margin: 0 0 8px; }
.quote .label { display: block; font-style: normal; font-size: .6rem; text-transform: uppercase; color: var(--muted); }

/* ---------------- spells ---------------- */
.spell-head { display: flex; flex-wrap: wrap; gap: 10px; margin-bottom: 10px; }
.slot-row { display: flex; align-items: center; gap: 6px; margin: 4px 0 6px; }
.slot-row .lvl { font-family: Cinzel, serif; font-size: .68rem; color: var(--muted); width: 3.2em; }
.slot { width: 12px; height: 12px; border: 1px solid var(--muted); border-radius: 3px; background: var(--paper); }
.slot.used { background: var(--muted); }
.spell-level { margin-bottom: 12px; }
.spell-level h3 {
  font-size: .74rem; text-transform: uppercase; color: var(--accent);
  border-bottom: 1px solid var(--line); margin: 0 0 4px; padding-bottom: 2px;
}
.prep { color: var(--accent-2); font-weight: 700; }
details.spell summary {
  cursor: pointer; font-size: .88rem; padding: 1px 0;
  border-bottom: 1px dotted rgba(139,126,102,.4);
}
details.spell summary::marker { color: var(--muted); }
details.spell p { font-size: .82rem; margin: 4px 0 8px 12px; white-space: pre-wrap; }
.tagline { font-size: .68rem; color: var(--muted); }
footer.sheet-foot {
  margin-top: 20px; padding-top: 8px; border-top: 1px solid var(--line);
  font-size: .68rem; color: var(--muted); display: flex; justify-content: space-between; gap: 12px;
}
.warn { color: var(--accent); font-size: .78rem; }

@media print {
  body { background: #fff; }
  .page { box-shadow: none; max-width: none; padding: 0; }
  .scroller { max-height: none; overflow: visible; }
  .box { break-inside: avoid; }
  details.spell { break-inside: avoid; }
  details.spell[open] summary ~ * { display: block; }
}
</style>
</head>
<body>
<div class="page">

  <header class="sheet-head">
    <div class="name-block">
      <h1 class="char-name">{{ sheet.name }}</h1>
      <div class="char-sub">{{ sheet.classLine }} &middot; {{ sheet.raceName }}{% if sheet.background %} &middot; {{ sheet.background }}{% endif %}</div>
    </div>
    <div class="head-facts">
      <div class="fact"><span class="label">Level</span><span class="value">{{ sheet.level }}</span></div>
      <div class="fact"><span class="label">Proficiency</span><span class="value">{{ sheet.proficiencyStr }}</span></div>
      {% if sheet.alignment %}<div class="fact"><span class="label">Alignment</span><span class="value">{{ sheet.alignment }}</span></div>{% endif %}
      <div class="fact"><span class="label">Experience</span><span class="value">{{ sheet.xp }}{% if sheet.nextLevelXp %} / {{ sheet.nextLevelXp }}{% endif %}</span></div>
      {% if sheet.player %}<div class="fact"><span class="label">Player</span><span class="value">{{ sheet.player }}</span></div>{% endif %}
      <div class="fact"><span class="label">Inspiration</span><span class="value">{% if sheet.inspiration %}Yes{% else %}&mdash;{% endif %}</span></div>
    </div>
  </header>

  <div class="columns">

    <!-- =============== left column =============== -->
    <div class="col">
      <div class="box">
        <h2>Ability Scores</h2>
        <div class="abilities">
          {% for a in sheet.abilities %}
          <div class="ability">
            <span class="tile-label">{{ a.short }}</span>
            <div class="mod">{{ a.mod }}</div>
            <span class="score">{{ a.score }}</span>
          </div>
          {% endfor %}
        </div>
      </div>

      <div class="box">
        <h2>Saving Throws</h2>
        <div class="rows">
          {% for a in sheet.abilities %}
          <div class="row">
            <span class="pip{% if a.saveProf %} on{% endif %}"></span>
            <span class="nm">{{ a.name }}</span>
            <span class="val">{{ a.saveStr }}</span>
          </div>
          {% endfor %}
        </div>
      </div>

      <div class="box">
        <h2>Skills</h2>
        <div class="rows">
          {% for s in sheet.skills %}
          <div class="row">
            <span class="pip{% if s.expertise %} exp{% elif s.proficient %} on{% endif %}"></span>
            <span class="abbr">{{ s.short }}</span>
            <span class="nm">{{ s.name }}</span>
            <span class="val">{{ s.mod }}</span>
          </div>
          {% endfor %}
        </div>
      </div>

      <div class="box">
        <h2>Passive Senses</h2>
        <div class="rows">
          <div class="row"><span class="nm">Passive Perception</span><span class="val">{{ sheet.passivePerception }}</span></div>
          <div class="row"><span class="nm">Passive Insight</span><span class="val">{{ sheet.passiveInsight }}</span></div>
          <div class="row"><span class="nm">Passive Investigation</span><span class="val">{{ sheet.passiveInvestigation }}</span></div>
          {% if sheet.darkvision %}<div class="row"><span class="nm">Darkvision</span><span class="val">{{ sheet.darkvision }}</span></div>{% endif %}
        </div>
      </div>

      <div class="box">
        <h2>Proficiencies &amp; Languages</h2>
        <div class="rows">
          <div class="row"><span class="nm"><strong>Armor</strong> {% if sheet.armorProficiencies %}{{ sheet.armorProficiencies|join:", " }}{% else %}none{% endif %}</span></div>
          <div class="row"><span class="nm"><strong>Weapons</strong> {% if sheet.weaponProficiencies %}{{ sheet.weaponProficiencies|join:", " }}{% else %}none{% endif %}</span></div>
          <div class="row"><span class="nm"><strong>Tools</strong> {% if sheet.toolProficiencies %}{{ sheet.toolProficiencies|join:", " }}{% else %}none{% endif %}</span></div>
          <div class="row"><span class="nm"><strong>Languages</strong> {% if sheet.languages %}{{ sheet.languages|join:", " }}{% else %}none{% endif %}</span></div>
        </div>
      </div>
    </div>

    <!-- =============== middle column =============== -->
    <div class="col">
      <div class="box">
        <h2>Combat</h2>
        <div class="combat">
          <div class="tile shield">
            <span class="tile-label">Armor Class</span>
            <div class="big">{{ sheet.ac }}</div>
            <span class="sub">{{ sheet.acSource }}</span>
          </div>
          <div class="tile">
            <span class="tile-label">Initiative</span>
            <div class="big">{{ sheet.initiativeStr }}</div>
            <span class="sub">dexterity</span>
          </div>
          <div class="tile">
            <span class="tile-label">Speed</span>
            <div class="big">{{ sheet.speed }}</div>
            <span class="sub">feet &middot; {{ sheet.size }}</span>
          </div>
        </div>
        <div style="margin-top:10px">
          <div class="row" style="border:0">
            <span class="nm"><strong>Hit Points</strong> {{ sheet.hpCurrent }} / {{ sheet.hpMax }}{% if sheet.hpTemp %} (+{{ sheet.hpTemp }} temp){% endif %}</span>
            <span class="val">{{ sheet.hitDice }}</span>
          </div>
          <div class="hp-bar"><div class="hp-fill" style="width:{{ sheet.hpPercent }}%"></div></div>
          <div class="tagline" style="margin-top:4px">
            Hit dice {{ sheet.hitDice }}{% if sheet.hitDiceUsed %}, {{ sheet.hitDiceUsed }} spent{% endif %} &middot;
            death saves {{ sheet.deathSaves }} &middot;
            carrying {{ sheet.weight }} of {{ sheet.carryCapacity }} lb
          </div>
        </div>
      </div>

      <div class="box">
        <h2>Attacks</h2>
        <div class="table-wrap">
        <table>
          <thead><tr><th>Attack</th><th class="num">Bonus</th><th>Damage</th><th>Range</th></tr></thead>
          <tbody>
          {% for a in sheet.attacks %}
            <tr>
              <td>{{ a.name }}{% if a.notes %}<div class="tagline">{{ a.notes }}</div>{% endif %}</td>
              <td class="num">{{ a.bonus }}</td>
              <td>{{ a.damage }} {{ a.type }}</td>
              <td>{{ a.range }}</td>
            </tr>
          {% endfor %}
          </tbody>
        </table>
        </div>
      </div>

      <div class="box">
        <h2>Equipment</h2>
        <div class="table-wrap">
        <table>
          <thead><tr><th>Item</th><th class="num">Qty</th><th class="num">Wt</th><th>Worn</th></tr></thead>
          <tbody>
          {% for g in sheet.equipment %}
            <tr>
              <td>{{ g.name }}{% if g.notes %} <span class="tagline">({{ g.notes }})</span>{% endif %}</td>
              <td class="num">{{ g.qty }}</td>
              <td class="num">{% if g.weight %}{{ g.weight }}{% endif %}</td>
              <td>{% if g.equipped %}&#10003;{% endif %}</td>
            </tr>
          {% endfor %}
          </tbody>
        </table>
        </div>
        <div class="tagline" style="margin-top:6px">
          {{ sheet.money.cp }} cp &middot; {{ sheet.money.sp }} sp &middot; {{ sheet.money.ep }} ep &middot;
          {{ sheet.money.gp }} gp &middot; {{ sheet.money.pp }} pp
        </div>
      </div>

      {% if sheet.personality or sheet.ideals or sheet.bonds or sheet.flaws %}
      <div class="box">
        <h2>Personality</h2>
        {% if sheet.personality %}<p class="quote"><span class="label">Traits</span>{{ sheet.personality }}</p>{% endif %}
        {% if sheet.ideals %}<p class="quote"><span class="label">Ideals</span>{{ sheet.ideals }}</p>{% endif %}
        {% if sheet.bonds %}<p class="quote"><span class="label">Bonds</span>{{ sheet.bonds }}</p>{% endif %}
        {% if sheet.flaws %}<p class="quote"><span class="label">Flaws</span>{{ sheet.flaws }}</p>{% endif %}
      </div>
      {% endif %}

      {% if sheet.age or sheet.height or sheet.weightStr or sheet.eyes or sheet.skin or sheet.hair or sheet.appearance %}
      <div class="box">
        <h2>Appearance</h2>
        <div class="rows">
          {% if sheet.age %}<div class="row"><span class="nm">Age</span><span class="val">{{ sheet.age }}</span></div>{% endif %}
          {% if sheet.height %}<div class="row"><span class="nm">Height</span><span class="val">{{ sheet.height }}</span></div>{% endif %}
          {% if sheet.weightStr %}<div class="row"><span class="nm">Weight</span><span class="val">{{ sheet.weightStr }}</span></div>{% endif %}
          {% if sheet.eyes %}<div class="row"><span class="nm">Eyes</span><span class="val">{{ sheet.eyes }}</span></div>{% endif %}
          {% if sheet.skin %}<div class="row"><span class="nm">Skin</span><span class="val">{{ sheet.skin }}</span></div>{% endif %}
          {% if sheet.hair %}<div class="row"><span class="nm">Hair</span><span class="val">{{ sheet.hair }}</span></div>{% endif %}
        </div>
        {% if sheet.appearance %}<p class="quote" style="margin-top:6px">{{ sheet.appearance }}</p>{% endif %}
      </div>
      {% endif %}
    </div>

    <!-- =============== right column =============== -->
    <div class="col">
      {% if sheet.isCaster %}
      <div class="box">
        <h2>Spellcasting</h2>
        <div class="spell-head">
          <div class="tile"><span class="tile-label">Ability</span><div class="big" style="font-size:1rem">{{ sheet.castingAbility }}</div></div>
          <div class="tile"><span class="tile-label">Save DC</span><div class="big">{{ sheet.spellSaveDc }}</div></div>
          <div class="tile"><span class="tile-label">Attack</span><div class="big">{{ sheet.spellAttackStr }}</div></div>
        </div>
        <div class="tagline">
          {% if sheet.cantripsKnown %}{{ sheet.cantripsKnown }} cantrips{% endif %}
          {% if sheet.spellsKnown %} &middot; {{ sheet.spellsKnown }} spells known{% endif %}
          {% if sheet.preparedMax %} &middot; {{ sheet.spellsPrepared }}/{{ sheet.preparedMax }} prepared{% endif %}
          {% if sheet.spellNotes %} &middot; {{ sheet.spellNotes }}{% endif %}
        </div>
        {% for s in sheet.slots %}
        <div class="slot-row">
          <span class="lvl">{{ s.level }}{% if s.level == 1 %}st{% elif s.level == 2 %}nd{% elif s.level == 3 %}rd{% else %}th{% endif %}</span>
          {% for i in s.pips %}<span class="slot{% if i <= s.used %} used{% endif %}"></span>{% endfor %}
          <span class="tagline">{{ s.total }} slot{% if s.total > 1 %}s{% endif %}</span>
        </div>
        {% endfor %}

        {% for lvl in sheet.spellLevels %}
        <div class="spell-level">
          <h3>{{ lvl.name }}{% if lvl.slots %} ({{ lvl.slots }} slot{% if lvl.slots > 1 %}s{% endif %}){% endif %}</h3>
          {% for sp in lvl.spells %}
          <details class="spell">
            <summary>
              {% if sp.prepared and lvl.level > 0 %}<span class="prep">&#9679;</span> {% endif %}{{ sp.name }}
              <span class="tagline">&mdash; {{ sp.castingTime }}, {{ sp.range }}{% if sp.concentration %}, concentration{% endif %}{% if sp.ritual %}, ritual{% endif %}</span>
            </summary>
            <p><em>{{ sp.school }}{% if sp.components %} &middot; {{ sp.components }}{% endif %}{% if sp.duration %} &middot; {{ sp.duration }}{% endif %}</em>
{{ sp.text }}{% if sp.higherLevel %}

<strong>At higher levels.</strong> {{ sp.higherLevel }}{% endif %}</p>
          </details>
          {% endfor %}
        </div>
        {% endfor %}
      </div>
      {% endif %}

      <div class="box">
        <h2>Features &amp; Traits</h2>
        <div class="scroller">
          {% for t in sheet.traits %}
          <div class="feature">
            <h3>{{ t.name }}</h3>
            {% if t.source %}<span class="src">{{ t.source }}</span>{% endif %}
            <p>{{ t.text }}</p>
          </div>
          {% endfor %}
          {% for f in sheet.features %}
          <div class="feature">
            <h3>{{ f.name }}</h3>
            {% if f.source %}<span class="src">{{ f.source }}</span>{% endif %}
            <p>{{ f.text }}</p>
          </div>
          {% endfor %}
        </div>
      </div>

      {% if sheet.backstory or sheet.allies or sheet.treasure or sheet.notes %}
      <div class="box">
        <h2>Journal</h2>
        {% if sheet.backstory %}<p class="quote"><span class="label">Backstory</span>{{ sheet.backstory }}</p>{% endif %}
        {% if sheet.allies %}<p class="quote"><span class="label">Allies &amp; Organizations</span>{{ sheet.allies }}</p>{% endif %}
        {% if sheet.treasure %}<p class="quote"><span class="label">Treasure</span>{{ sheet.treasure }}</p>{% endif %}
        {% if sheet.notes %}<p class="quote"><span class="label">Notes</span>{{ sheet.notes }}</p>{% endif %}
      </div>
      {% endif %}
    </div>
  </div>

  {% if sheet.warnings %}
  <div class="box" style="margin-top:16px">
    <h2>Sheet Warnings</h2>
    {% for w in sheet.warnings %}<div class="warn">{{ w }}</div>{% endfor %}
  </div>
  {% endif %}

  <footer class="sheet-foot">
    <span>{{ sheet.name }} &middot; {{ sheet.classLine }} &middot; {{ sheet.raceName }}</span>
    <span>generated by orgs &middot; {{ sheet.rulesetName }}</span>
  </footer>
</div>
</body>
</html>
