'use strict';

jQuery('#loginForm').submit(function(e) {
	jQuery(this).find('button[type="submit"]').attr('disabled', 'disabled').html('Logging In...');
});
