package repository

type User struct {
	ID    string
	Email string
	Name  string
}

type userRepository struct {
	db *DatabaseFixture
}

func newUserRepository(db *DatabaseFixture) *userRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(u User) {
	r.db.Put("user:"+u.ID, u)
}

func (r *userRepository) FindByID(id string) (User, bool) {
	v, ok := r.db.Get("user:" + id)
	if !ok {
		return User{}, false
	}
	return v.(User), true
}

func (r *userRepository) Delete(id string) {
	r.db.Delete("user:" + id)
}
