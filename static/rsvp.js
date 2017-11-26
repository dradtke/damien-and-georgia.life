'use strict';

var $form = jQuery('#rsvpForm');

$form.submit(function() {
	jQuery('button[type="submit"]').attr('disabled', 'disabled').html('Saving...');
});

window.onload = function() {
	if ($form.length > 0) $form[0].reset();
}

jQuery('input[name="Attending"]').change(fixTheWorld);
jQuery('input[name="PlusOne"]').change(fixTheWorld);

function fixTheWorld() {
	var attending = jQuery('input[name="Attending"]:checked').val() === 'yes';
	var plusOne = jQuery('input[name="PlusOne"]:checked').val() === 'yes';

	jQuery('#groupPlusOne').toggleClass('d-none', !attending);
	jQuery('#groupPlusOneName').toggleClass('d-none', !(attending && plusOne));

	if (attending) {
		jQuery('input[name="PlusOne"]').attr('required', 'required');
	} else {
		jQuery('input[name="PlusOne"]').removeAttr('required');
	}

	if (attending && plusOne) {
		jQuery('input[name="PlusOneName"]').attr('required', 'required');
	} else {
		jQuery('input[name="PlusOneName"]').removeAttr('required');
	}
}
