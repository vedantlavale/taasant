// Writes the number of GitHub stars into every .stars on the page. GitHub
// answers 60 of these questions an hour for one visitor, so the number is
// kept for an hour. Until it is known, or if GitHub cannot be reached, the
// text already in the page stays.
(function () {
  var spots = document.querySelectorAll('.stars');
  function show(count) {
    var text = count < 1000 ? String(count) : (count / 1000).toFixed(1) + 'k';
    spots.forEach(function (spot) { spot.innerHTML = '<span class="star">★</span> ' + text; });
  }

  var saved = null;
  try { saved = JSON.parse(localStorage.getItem('taasant-stars')); } catch (e) {}
  if (saved) show(saved.count);
  if (saved && Date.now() - saved.at < 3600000) return;

  fetch('https://api.github.com/repos/vedantlavale/taasant')
    .then(function (answer) { return answer.json(); })
    .then(function (repo) {
      if (typeof repo.stargazers_count !== 'number') return;
      show(repo.stargazers_count);
      try { localStorage.setItem('taasant-stars', JSON.stringify({ count: repo.stargazers_count, at: Date.now() })); } catch (e) {}
    })
    .catch(function () {});
})();
