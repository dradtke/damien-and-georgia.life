'use strict';

var pictures = [
	'12143351_10207282122748594_7296485508454759107_n.jpg',
	'11892136_10100501027220870_2977168626840559920_n.jpg',
	'11150711_10206032039417292_1988554892302920147_n.jpg',
	'15350602_10210416944517179_5078769292854320016_n.jpg',
	'526828_10151555714703240_1895636344_n.jpg'
];

function displayImage(i) {
	$('#currentImage').html('<img class="img-fluid" src="/static/pictures/' + pictures[i] + '">');
}

var currentIndex = 0;
displayImage(currentIndex);

window.onkeyup = function(e) {
	switch (e.keyCode) {
	case 39: // Right
		currentIndex = Math.min(currentIndex+1, pictures.length-1);
		displayImage(currentIndex);
		break;
	case 37: // Left
		currentIndex = Math.max(currentIndex-1, 0);
		displayImage(currentIndex);
		break;
	default:
		// console.log(e.keyCode);
	}
};
