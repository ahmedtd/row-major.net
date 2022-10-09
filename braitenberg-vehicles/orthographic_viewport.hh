
#ifndef ORTHOGRAPHIC_VIEWPORT_HH
#define ORTHOGRAPHIC_VIEWPORT_HH

#include <armadillo>
using arma::vec;

class orthographic_viewport
{
public:
    orthographic_viewport(const vec &screen_size,
                          const double &pixels_per_meter,
                          const vec &center,
                          const vec &direction);

    double pixels_per_meter() const;

    void pixels_per_meter(double newval);

    vec screen_size() const;

    void screen_size(const vec &newval);
    vec direction() const;

    void direction(const vec &newval);

    vec center() const;

    void center(const vec &newval);
    void use();

    void draw_cartesian_grid() const;
    
    void grid_state(bool show);
    bool grid_state() const;
private:
    vec m_screen_size;

    double m_pixels_per_meter;

    vec m_center;
    vec m_direction;

    bool m_grid_shown;
};

#endif
