
#ifndef LIGHT_DISPLAY_HH
#define LIGHT_DISPLAY_HH

#include <vector>
using std::vector;

#include <armadillo>
using arma::mat;
using arma::vec;

//#include "bounding_box"

// Display halves need to have formal methods to accept state from their
// corresponding models.  In this way, both halves will be able to have actions
// and updates that are only undertaken every time a frame is scheduled to be
// rendered, regardless of whether or not the display halve will have render()
// called on this frame or not.
class light_display
{
    friend class light_model;

public:
    light_display();

    light_display(
        const vec &position,
        const double &intensity
    );

    // Render may have side effects due to animation computation, caching, etc.
    //
    // While the object is "const" in comparison to the model, it is not const
    // in a literal sense.
    void render();

    //const bbox<double>& extents() const;

private:
    vec m_position;
    double m_intensity;
    vec m_color;

    // Percent of the way through our animation cycle
    vector<double> m_anim_times;

    // bbox<double> m_extents;

    // This is asynchronously updated by our controlling light_model.
    double m_elapsed_time;

    // Caching for fast drawing of circles
    static mat sm_unit_circle_table;
};

#endif
