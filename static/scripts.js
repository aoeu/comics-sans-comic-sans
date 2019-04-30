$(document).ready(function() {

var AllSeries;

$.getJSON('comics.json', function(response){
    AllSeries = response;
    console.log(response);
});

$('a[href^="#"]').bind('click.smoothscroll',function (e) {
    e.preventDefault();
    var target = this.hash;
    $target = $(target);
    $('html, body').stop().animate({
        'scrollTop': $target.offset().top
    }, 250, 'swing', function () {
        window.location.hash = target;
    });
});

function redraw(series, seriesID) {   
    comic = series.Comics[series.Index];
    var dateText = comic.PubMsg + " " + comic.Date;
    $('#date_'    + seriesID).text(dateText);
    $('#link_'    + seriesID).attr('href', comic.Link);
    $('#img_'     + seriesID).attr('src', comic.ImageURL);

    $('#title_' + seriesID).fadeOut(333, function() {
        $('#title_'   + seriesID).text(comic.Title);
        $('#title_' + seriesID).fadeIn(333);
    });

    $('#comment_' + seriesID).fadeOut(333, function() {
        $('#comment_' + seriesID).text(comic.ImageComment);
        $('#comment_' + seriesID).fadeIn(333);
    });
}

function changeTitle(id, newText) {
    var title = $('#title_' + id)
    title.fadeOut(333, function() {
        $(this).text(newText);
        $(this).fadeIn(333, function() { });
    });
}

function updateImages(id, delta){ 
    series = AllSeries[parseInt(id, 10)];
    if (series.Index + delta > series.Comics.length - 1) {
        changeTitle(id, "No older comics in feed. (Click on the series title and visit the site's archive.)");
        return;
    } else if(series.Index + delta < 0) {
        changeTitle(id, "No newer comics in feed.");
        return;
    } else {
        series.Index += delta;
    }
    var img = $('#img_' + id);
    img.parents('.image').css('height', img.height()); // HACK
    img.fadeOut(777, function() {
        redraw(series, id); 
        $(this).load(function() { 
            $('#image_' + id).animate({
                height: $(this).height()
            }, 333);
            $(this).fadeIn(777, function() { });
        } );
    });
}

$('.prev').click(function(){ updateImages( $(this).attr('id'),  1); });
$('.next').click(function(){ updateImages( $(this).attr('id'), -1); });

})
