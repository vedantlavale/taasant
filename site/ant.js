// The ant above the footer. Five times a second it does one small thing, so
// it moves in jumps like an old game. When it finishes what it was doing it
// picks the next thing by chance. Someone who asked their system for less
// motion keeps the ant that stands still.
(function () {
  var track = document.querySelector('.ant-track');
  if (!track || matchMedia('(prefers-reduced-motion: reduce)').matches) return;

  // The logo, with its antennae and two sets of legs in groups of their own.
  track.innerHTML = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 21 11" shape-rendering="crispEdges"><g class="antenna"><rect x="16" y="0" width="1" height="1" fill="#6d5df6"/><rect x="19" y="0" width="1" height="1" fill="#6d5df6"/><rect x="15" y="1" width="1" height="1" fill="#6d5df6"/><rect x="18" y="1" width="1" height="1" fill="#6d5df6"/></g><rect x="1" y="2" width="6" height="1" fill="#6d5df6"/><rect x="14" y="2" width="6" height="1" fill="#6d5df6"/><rect x="0" y="3" width="8" height="1" fill="#6d5df6"/><rect x="13" y="3" width="8" height="1" fill="#6d5df6"/><rect x="0" y="4" width="8" height="1" fill="#6d5df6"/><rect x="9" y="4" width="4" height="1" fill="#6d5df6"/><rect x="14" y="4" width="2" height="1" fill="#6d5df6"/><rect x="16" y="4" width="1" height="1" fill="#15122e"/><rect x="17" y="4" width="2" height="1" fill="#6d5df6"/><rect x="19" y="4" width="1" height="1" fill="#15122e"/><rect x="20" y="4" width="1" height="1" fill="#6d5df6"/><rect x="0" y="5" width="16" height="1" fill="#6d5df6"/><rect x="16" y="5" width="1" height="1" fill="#15122e"/><rect x="17" y="5" width="2" height="1" fill="#6d5df6"/><rect x="19" y="5" width="1" height="1" fill="#15122e"/><rect x="20" y="5" width="1" height="1" fill="#6d5df6"/><rect x="0" y="6" width="8" height="1" fill="#6d5df6"/><rect x="9" y="6" width="4" height="1" fill="#6d5df6"/><rect x="14" y="6" width="7" height="1" fill="#6d5df6"/><rect x="1" y="7" width="6" height="1" fill="#6d5df6"/><rect x="9" y="7" width="4" height="1" fill="#6d5df6"/><rect x="15" y="7" width="5" height="1" fill="#6d5df6"/><g class="a"><rect x="8" y="8" width="1" height="1" fill="#6d5df6"/><rect x="10" y="8" width="2" height="1" fill="#6d5df6"/><rect x="13" y="8" width="1" height="1" fill="#6d5df6"/><rect x="7" y="9" width="1" height="1" fill="#6d5df6"/><rect x="10" y="9" width="2" height="1" fill="#6d5df6"/><rect x="14" y="9" width="1" height="1" fill="#6d5df6"/><rect x="7" y="10" width="1" height="1" fill="#6d5df6"/><rect x="10" y="10" width="2" height="1" fill="#6d5df6"/><rect x="14" y="10" width="1" height="1" fill="#6d5df6"/></g><g class="b"><rect x="8" y="8" width="1" height="1" fill="#6d5df6"/><rect x="10" y="8" width="2" height="1" fill="#6d5df6"/><rect x="13" y="8" width="1" height="1" fill="#6d5df6"/><rect x="8" y="9" width="1" height="1" fill="#6d5df6"/><rect x="10" y="9" width="2" height="1" fill="#6d5df6"/><rect x="13" y="9" width="1" height="1" fill="#6d5df6"/><rect x="9" y="10" width="1" height="1" fill="#6d5df6"/><rect x="11" y="10" width="2" height="1" fill="#6d5df6"/><rect x="12" y="10" width="1" height="1" fill="#6d5df6"/></g></svg>';
  (function () {
    var ant = track.firstElementChild;
    var x = 24, up = 0, facing = 1;      // facing is 1 for right, -1 for left
    var doing = 'stand', ticks = 5, target = 0;

    function random(from, to) { return from + Math.floor(Math.random() * (to - from + 1)); }

    function choose() {
      // Walking is in the list more than once, so it is picked most often.
      doing = ['walk', 'walk', 'walk', 'run', 'stand', 'look', 'hop', 'twitch'][random(0, 7)];
      ticks = random(6, 16);
      if (doing === 'walk' || doing === 'run') {
        target = random(0, Math.max(0, track.clientWidth - 42));
        facing = target < x ? -1 : 1;
        ticks = 1000;                    // it stops when it gets there instead
      }
    }

    setInterval(function () {
      ticks--;
      if (ticks <= 0) choose();
      up = 0;
      ant.classList.remove('twitch');

      if (doing === 'walk' || doing === 'run') {
        var pace = doing === 'run' ? 16 : 6;
        if (Math.abs(target - x) <= pace) { x = target; ticks = 0; }
        else x += facing * pace;
        ant.classList.toggle('alt');
      }
      if (doing === 'look' && ticks % 4 === 0) facing = -facing;
      if (doing === 'hop' && ticks % 2 === 0) up = 8;
      if (doing === 'twitch' && ticks % 2 === 0) ant.classList.add('twitch');

      ant.style.transform = 'translate(' + x + 'px, ' + -up + 'px) scaleX(' + facing + ')';
    }, 200);
  })();
})();
