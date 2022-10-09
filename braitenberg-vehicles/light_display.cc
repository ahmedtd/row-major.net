
#include "light_display.hh"

#include <cmath>
using std::exp;

#include <GL/gl.h>

#include <armadillo>
using arma::mat;
using arma::vec;
using arma::datum;

#include "logistic.hh"

mat light_display::sm_unit_circle_table;

light_display::light_display()
{
}

light_display::light_display(
    const vec &position,
    const double &intensity
)
    : m_position(position),
      m_intensity(intensity),
      m_anim_times{0.0, 0.33, 0.66},
      // m_extents{
      //     position(1) + 0.5,
      //     position(0) - 0.5,
      //     position(1) - 0.5,
      //     position(0) + 0.5
      // },
      m_elapsed_time(0.0)
{
    // Set the color based purely on intensity.  If color were animated, we
    // would update it in the render method below.
    m_color << logistic(-intensity) // red
            << 0.2                  // green
            << logistic(intensity)  // blue
            << 0.2;

    if(sm_unit_circle_table.n_cols != 60)
    {
        sm_unit_circle_table.set_size(2, 60);
        
        double cur_angle = 0;
        for(unsigned int count = 0;
            count < sm_unit_circle_table.n_cols;
            count++)
        {
            sm_unit_circle_table(0, count) = cur_angle;
            sm_unit_circle_table(1, count) = cur_angle;

            cur_angle += 2*datum::pi / 60.0;
        }

        sm_unit_circle_table.row(0) = cos(sm_unit_circle_table.row(0));
        sm_unit_circle_table.row(1) = sin(sm_unit_circle_table.row(1));
    }
}

void light_display::render()
{
    const double anim_max = 1.0;

    // Perform animation update
    for(auto &anim_time : m_anim_times)
    {
        anim_time += m_elapsed_time;
        if(anim_time > anim_max)
            anim_time = 0.0;
    }
    
    // Do once for each animation parameter.
    for(const auto &anim_time : m_anim_times)
    {
        double radius = 0.0;
        if(m_intensity > 0)
        {
            radius = (anim_time / anim_max * 0.5);
            m_color(3) = 1.0 * (anim_max - anim_time) / anim_max;
        }
        else
        {
            radius = (anim_max - anim_time) / anim_max * 0.5;
            m_color(3) = 1.0 * (anim_time) / anim_max;
        }

        glMatrixMode(GL_MODELVIEW);
    
        glPushMatrix();
        glTranslated(m_position(0), m_position(1), 0);

        glBegin(GL_TRIANGLE_FAN);
    
        glColor4d(m_color(0), m_color(1), m_color(2), m_color(3));

        glVertex2d(0,0);
        for(unsigned int count = 0;
            count < sm_unit_circle_table.n_cols;
            count++)
        {
            glVertex2d(radius * sm_unit_circle_table(0, count),
                       radius * sm_unit_circle_table(1, count));
        }
        glVertex2d(radius * sm_unit_circle_table(0, 0),
                   radius * sm_unit_circle_table(1, 0));
    
        glEnd();

        glPopMatrix();
    }
}
