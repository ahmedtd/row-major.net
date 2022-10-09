
#ifndef BOUNDING_BOX_HH
#define BOUNDING_BOX_HH

template<class Real>
struct bbox
{
    Real top;
    Real left;
    Real bottom;
    Real right;
};

template<class Real>
bool overlaps(const bbox<Real> &a, const bbox<Real> &b)
{
    return
        (a.top <= b.top && a.left >= b.left
         && a.top >= b.bottom && a.left <= b.right)
        || (a.bottom >= b.bottom && a.right <= b.right
            && a.bottom <= b.top && a.right >= b.left);
}

#endif
