'use strict';

var $form = jQuery('#rsvpForm');

window.onload = function() {
	$form[0].reset();
}

jQuery('input[name="Attending"]').change(function() {
	var $this = jQuery(this);
	jQuery('#groupPlusOne').toggleClass('d-none', $this.val() !== 'yes');
});

jQuery('input[name="PlusOne"]').change(function() {
	var $this = jQuery(this);
	jQuery('#groupPlusOneName').toggleClass('d-none', !$this.is(':checked'));
	if ($this.is(':checked')) {
		jQuery('#formPlusOneName').attr('required', 'required');
	} else {
		jQuery('#formPlusOneName').removeAttr('required');
	}
});
