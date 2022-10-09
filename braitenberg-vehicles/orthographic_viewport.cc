#include "orthographic_viewport.hh"

#include <cmath>
using std::atan2;
// #include <iostream>
// using std::cout;
// using std::endl;

#include <armadillo>
using arma::datum;

#include <GL/freeglut.h>

orthographic_viewport::orthographic_viewport(
    const vec &screen_size,
    const double &pixels_per_meter,
    const vec &center,
    const vec &direction)
    :
    m_screen_size(screen_size),
    m_pixels_per_meter(pixels_per_meter),
    m_center(center),
    m_direction(direction),
    m_grid_shown(false)
{
    
}

double orthographic_viewport::pixels_per_meter() const
{
    return m_pixels_per_meter;
}

void orthographic_viewport::pixels_per_meter(double newval)
{
    m_pixels_per_meter = newval;
}

vec orthographic_viewport::screen_size() const
{
    return m_screen_size;
}

void orthographic_viewport::screen_size(const vec &newval)
{
    m_screen_size = newval;
}

vec orthographic_viewport:: direction() const
{
    return m_direction;
}

void orthographic_viewport::direction(const vec &newval)
{
    m_direction = newval;
}

vec orthographic_viewport::center() const
{
    return m_center;
}

void orthographic_viewport::center(const vec &newval)
{
    m_center = newval;
}

void orthographic_viewport::use()
{
    int prev_mode;
    glGetIntegerv(GL_MATRIX_MODE, &prev_mode);

    glMatrixMode(GL_PROJECTION);

    glLoadIdentity();
    glOrtho(m_center(0)-(m_screen_size(0))/m_pixels_per_meter/2.0,
            m_center(0)+(m_screen_size(0))/m_pixels_per_meter/2.0,
            m_center(1)-(m_screen_size(1))/m_pixels_per_meter/2.0,
            m_center(1)+(m_screen_size(1))/m_pixels_per_meter/2.0,
            -1.0,
            1.0);

    glRotated(atan2(m_direction(1), m_direction(0))*180.0/datum::pi,
              0,
              0,
              1);

    glMatrixMode(prev_mode);
}

void orthographic_viewport::draw_cartesian_grid() const
{
    if(! m_grid_shown)
        return;

    vec br = m_screen_size / m_pixels_per_meter / 2.0;        
        
    // Install a scale-only projection so that the grid is independent of
    // the current view translation and rotation.
        
    int prev_mode;
    glGetIntegerv(GL_MATRIX_MODE, &prev_mode);

    glMatrixMode(GL_PROJECTION);
    glPushMatrix();
    glLoadIdentity();
    glOrtho(-(m_screen_size(0))/m_pixels_per_meter/2.0,
            (m_screen_size(0))/m_pixels_per_meter/2.0,
            -(m_screen_size(1))/m_pixels_per_meter/2.0,
            (m_screen_size(1))/m_pixels_per_meter/2.0,
            -1.0,
            1.0);

    for(unsigned int count = 0; count * 0.25 < br(0); count++)
    {
        if(count % 4 == 0)
            glColor4f(1, 1, 1, 0.9);
        else
            glColor4f(1, 1, 1, 0.4);

        glBegin(GL_LINES);
            
        glVertex2d(count * 0.25, -br(1));
        glVertex2d(count * 0.25, br(1));

        glVertex2d(count * (-0.25), -br(1));
        glVertex2d(count * (-0.25), br(1));
            
        glEnd();
    }

    for(unsigned int count = 0; count * 0.25 < br(1); count++)
    {
        if(count % 4 == 0)
            glColor4f(1, 1, 1, 0.9);
        else
            glColor4f(1, 1, 1, 0.4);

        glBegin(GL_LINES);
            
        glVertex2d(-br(0), count * 0.25);
        glVertex2d( br(0), count * 0.25);
            
        glVertex2d(-br(0), count * (-0.25));
        glVertex2d( br(0), count * (-0.25));

        glEnd();
    }

    glPopMatrix();
    glMatrixMode(prev_mode);
}

void orthographic_viewport::grid_state(bool show)
{
    m_grid_shown = show;
}

bool orthographic_viewport::grid_state() const
{
    return m_grid_shown;
}
